package stealth

import (
	"encoding/base32"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// DNSTunnel implements tunneling over DNS queries
// This is the LAST RESORT transport - works even when ONLY DNS (UDP/53) is allowed
//
// How it works:
// 1. Client encodes data as DNS subdomain queries:
//    "encoded-data.tunnel.yourdomain.com" → DNS server
// 2. DNS server (your server) decodes the subdomain and processes the data
// 3. Response is encoded in DNS TXT/CNAME/NULL records
//
// Advantages:
// - Works when EVERYTHING else is blocked (only needs UDP/53)
// - DNS is NEVER fully blocked (breaks all internet)
// - Very hard to detect (looks like normal DNS)
//
// Disadvantages:
// - Very slow (50-100 KB/s max)
// - High latency
// - Use only as absolute last resort
//
// In Iran's 90-day shutdown: DNS (UDP/53) was the ONLY protocol that survived
type DNSTunnel struct {
	domain     string // Your domain (e.g., "t.yourdomain.com")
	serverIP   string // DNS server IP
	log        *logger.Logger
	maxLabelLen int   // Max DNS label length (63)
	mtu        int
	mu         sync.Mutex
}

// DNSTunnelConfig holds DNS tunnel configuration
type DNSTunnelConfig struct {
	Domain   string // Base domain for tunnel (e.g., "t.example.com")
	ServerIP string // IP of your authoritative DNS server
	MTU      int    // MTU for DNS tunnel (default: 200)
}

// NewDNSTunnel creates a new DNS tunnel
func NewDNSTunnel(cfg DNSTunnelConfig, log *logger.Logger) *DNSTunnel {
	if cfg.MTU == 0 {
		cfg.MTU = 200 // Conservative for DNS
	}

	return &DNSTunnel{
		domain:      cfg.Domain,
		serverIP:    cfg.ServerIP,
		log:         log,
		maxLabelLen: 63,
		mtu:         cfg.MTU,
	}
}

// Send sends data through DNS queries
// Data is encoded as: base32(data).sequence.session.domain
func (dt *DNSTunnel) Send(data []byte, sessionID string, seq int) error {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Encode data as base32 (DNS-safe characters)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(data)
	encoded = strings.ToLower(encoded)

	// Split into DNS labels (max 63 chars each)
	labels := splitIntoLabels(encoded, dt.maxLabelLen-5) // Leave room for metadata

	// Build DNS query name
	// Format: label1.label2.seq.session.domain
	queryParts := append(labels, fmt.Sprintf("%d", seq), sessionID, dt.domain)
	queryName := strings.Join(queryParts, ".")

	if len(queryName) > 253 { // Max DNS name length
		return fmt.Errorf("DNS query too long: %d chars", len(queryName))
	}

	// Send DNS query
	return dt.sendDNSQuery(queryName)
}

// Receive receives data from DNS response
func (dt *DNSTunnel) Receive(sessionID string, seq int) ([]byte, error) {
	// Build query for receiving data
	queryName := fmt.Sprintf("r.%d.%s.%s", seq, sessionID, dt.domain)

	// Send query and get TXT record response
	response, err := dt.queryTXT(queryName)
	if err != nil {
		return nil, err
	}

	// Decode base32 response
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(response))
	if err != nil {
		return nil, fmt.Errorf("failed to decode DNS response: %w", err)
	}

	return decoded, nil
}

// sendDNSQuery sends a DNS A query
func (dt *DNSTunnel) sendDNSQuery(name string) error {
	// Use system resolver or direct UDP to server
	target := dt.serverIP + ":53"
	if dt.serverIP == "" {
		target = "8.8.8.8:53" // Fallback
	}

	conn, err := net.DialTimeout("udp", target, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Build DNS query packet
	query := buildDNSQueryPacket(name, 1) // Type A

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(query)
	return err
}

// queryTXT sends a DNS TXT query and returns the response
func (dt *DNSTunnel) queryTXT(name string) (string, error) {
	target := dt.serverIP + ":53"
	if dt.serverIP == "" {
		target = "8.8.8.8:53"
	}

	conn, err := net.DialTimeout("udp", target, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	// Build TXT query
	query := buildDNSQueryPacket(name, 16) // Type TXT

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(query); err != nil {
		return "", err
	}

	// Read response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	// Parse TXT record from response
	return parseTXTResponse(buf[:n])
}

// splitIntoLabels splits a string into DNS-safe labels
func splitIntoLabels(s string, maxLen int) []string {
	labels := make([]string, 0)
	for len(s) > 0 {
		end := maxLen
		if end > len(s) {
			end = len(s)
		}
		labels = append(labels, s[:end])
		s = s[end:]
	}
	return labels
}

// buildDNSQueryPacket builds a raw DNS query packet
func buildDNSQueryPacket(name string, qtype uint16) []byte {
	msg := make([]byte, 0, 512)

	// Header
	msg = append(msg, 0x12, 0x34) // Transaction ID
	msg = append(msg, 0x01, 0x00) // Flags: standard query, RD
	msg = append(msg, 0x00, 0x01) // Questions: 1
	msg = append(msg, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00) // Answers, Auth, Additional: 0

	// Question
	parts := strings.Split(name, ".")
	for _, part := range parts {
		msg = append(msg, byte(len(part)))
		msg = append(msg, []byte(part)...)
	}
	msg = append(msg, 0x00) // Root

	// Type and Class
	msg = append(msg, byte(qtype>>8), byte(qtype)) // Type
	msg = append(msg, 0x00, 0x01)                   // Class IN

	return msg
}

// parseTXTResponse extracts TXT record data from DNS response
func parseTXTResponse(data []byte) (string, error) {
	if len(data) < 12 {
		return "", fmt.Errorf("response too short")
	}

	// Skip header (12 bytes) and question section
	offset := 12
	// Skip question
	for offset < len(data) && data[offset] != 0 {
		if data[offset] >= 192 {
			offset += 2
			break
		}
		offset += int(data[offset]) + 1
	}
	if offset < len(data) && data[offset] == 0 {
		offset++
	}
	offset += 4 // Skip QTYPE + QCLASS

	// Parse answer
	if offset >= len(data) {
		return "", fmt.Errorf("no answer section")
	}

	// Skip answer name
	if data[offset] >= 192 {
		offset += 2
	}
	offset += 8 // Skip TYPE(2) + CLASS(2) + TTL(4)

	if offset+2 > len(data) {
		return "", fmt.Errorf("truncated answer")
	}

	rdLen := int(data[offset])<<8 | int(data[offset+1])
	offset += 2

	if offset+rdLen > len(data) {
		return "", fmt.Errorf("RDATA overflow")
	}

	// TXT record: first byte is string length
	if rdLen > 1 {
		txtLen := int(data[offset])
		offset++
		if offset+txtLen <= len(data) {
			return string(data[offset : offset+txtLen]), nil
		}
	}

	return "", fmt.Errorf("empty TXT record")
}

// EstimateBandwidth returns estimated bandwidth for DNS tunnel
func EstimateBandwidth() string {
	return "~50-100 KB/s (DNS tunnel is slow but works when everything else is blocked)"
}
