package dns

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Resolver implements DNS over HTTPS (DoH) to bypass DNS poisoning
// Iran's DPI poisons DNS responses for blocked domains
// DoH encrypts DNS queries inside HTTPS, making them invisible to DPI
type Resolver struct {
	servers []string // DoH server URLs
	client  *http.Client
	cache   sync.Map // DNS cache
	log     *logger.Logger
}

// DoHServer represents a DNS over HTTPS server
type DoHServer struct {
	URL  string
	Name string
}

// Well-known DoH servers
var DefaultDoHServers = []DoHServer{
	{URL: "https://1.1.1.1/dns-query", Name: "Cloudflare"},
	{URL: "https://8.8.8.8/dns-query", Name: "Google"},
	{URL: "https://9.9.9.9/dns-query", Name: "Quad9"},
	{URL: "https://208.67.222.222/dns-query", Name: "OpenDNS"},
	{URL: "https://185.228.168.168/dns-query", Name: "CleanBrowsing"},
}

// CacheEntry holds a cached DNS result
type CacheEntry struct {
	IPs       []net.IP
	ExpiresAt time.Time
}

// NewResolver creates a new DoH resolver
func NewResolver(servers []string, log *logger.Logger) *Resolver {
	if len(servers) == 0 {
		// Use defaults
		servers = make([]string, len(DefaultDoHServers))
		for i, s := range DefaultDoHServers {
			servers[i] = s.URL
		}
	}

	// Create HTTP client that doesn't use system DNS
	transport := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false,
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
	}

	return &Resolver{
		servers: servers,
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		log: log,
	}
}

// Resolve resolves a hostname using DoH
func (r *Resolver) Resolve(hostname string) ([]net.IP, error) {
	// Check cache first
	if entry, ok := r.cache.Load(hostname); ok {
		cached := entry.(*CacheEntry)
		if time.Now().Before(cached.ExpiresAt) {
			r.log.Debug("DNS cache hit: %s → %v", hostname, cached.IPs)
			return cached.IPs, nil
		}
		r.cache.Delete(hostname)
	}

	// Try each DoH server
	var lastErr error
	for _, server := range r.servers {
		ips, err := r.queryDoH(server, hostname)
		if err != nil {
			lastErr = err
			r.log.Debug("DoH query to %s failed: %v", server, err)
			continue
		}

		if len(ips) > 0 {
			// Cache result (5 minutes TTL)
			r.cache.Store(hostname, &CacheEntry{
				IPs:       ips,
				ExpiresAt: time.Now().Add(5 * time.Minute),
			})
			r.log.Debug("DNS resolved: %s → %v (via DoH)", hostname, ips)
			return ips, nil
		}
	}

	return nil, fmt.Errorf("all DoH servers failed for %s: %w", hostname, lastErr)
}

// queryDoH performs a DNS query using DNS over HTTPS (RFC 8484)
func (r *Resolver) queryDoH(serverURL, hostname string) ([]net.IP, error) {
	// Build DNS query message
	query := buildDNSQuery(hostname, 1) // Type A (IPv4)

	// Encode as base64url for GET request
	encoded := base64.RawURLEncoding.EncodeToString(query)

	url := fmt.Sprintf("%s?dns=%s", serverURL, encoded)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/dns-message")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("DoH server returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse DNS response
	return parseDNSResponse(body)
}

// ResolveAddr resolves a host:port address using DoH
func (r *Resolver) ResolveAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, nil // Already an IP or invalid
	}

	// Check if already an IP
	if ip := net.ParseIP(host); ip != nil {
		return addr, nil
	}

	ips, err := r.Resolve(host)
	if err != nil {
		return "", err
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IPs found for %s", host)
	}

	// Prefer IPv4
	for _, ip := range ips {
		if ip.To4() != nil {
			return net.JoinHostPort(ip.String(), port), nil
		}
	}

	return net.JoinHostPort(ips[0].String(), port), nil
}

// buildDNSQuery builds a minimal DNS query message
func buildDNSQuery(hostname string, qtype uint16) []byte {
	// DNS header (12 bytes)
	msg := make([]byte, 0, 512)

	// Transaction ID
	msg = append(msg, 0xAB, 0xCD)
	// Flags: standard query, recursion desired
	msg = append(msg, 0x01, 0x00)
	// Questions: 1
	msg = append(msg, 0x00, 0x01)
	// Answer, Authority, Additional: 0
	msg = append(msg, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)

	// Question section
	// Encode hostname as DNS labels
	labels := splitHostname(hostname)
	for _, label := range labels {
		msg = append(msg, byte(len(label)))
		msg = append(msg, []byte(label)...)
	}
	msg = append(msg, 0x00) // Root label

	// Type (A=1, AAAA=28)
	msg = append(msg, byte(qtype>>8), byte(qtype))
	// Class IN
	msg = append(msg, 0x00, 0x01)

	return msg
}

// parseDNSResponse parses a DNS response and extracts IP addresses
func parseDNSResponse(data []byte) ([]net.IP, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("DNS response too short")
	}

	// Parse header
	answerCount := int(data[6])<<8 | int(data[7])
	if answerCount == 0 {
		return nil, nil
	}

	// Skip question section
	offset := 12
	// Skip QNAME
	for offset < len(data) {
		length := int(data[offset])
		if length == 0 {
			offset++
			break
		}
		if length >= 192 { // Pointer
			offset += 2
			break
		}
		offset += length + 1
	}
	// Skip QTYPE and QCLASS
	offset += 4

	// Parse answers
	var ips []net.IP
	for i := 0; i < answerCount && offset < len(data); i++ {
		// Skip NAME (may be pointer)
		if offset >= len(data) {
			break
		}
		if data[offset] >= 192 { // Pointer
			offset += 2
		} else {
			for offset < len(data) {
				length := int(data[offset])
				if length == 0 {
					offset++
					break
				}
				offset += length + 1
			}
		}

		if offset+10 > len(data) {
			break
		}

		// TYPE
		rtype := int(data[offset])<<8 | int(data[offset+1])
		offset += 2

		// CLASS
		offset += 2

		// TTL
		offset += 4

		// RDLENGTH
		rdlength := int(data[offset])<<8 | int(data[offset+1])
		offset += 2

		if offset+rdlength > len(data) {
			break
		}

		// RDATA
		if rtype == 1 && rdlength == 4 { // A record
			ip := net.IPv4(data[offset], data[offset+1], data[offset+2], data[offset+3])
			ips = append(ips, ip)
		} else if rtype == 28 && rdlength == 16 { // AAAA record
			ip := make(net.IP, 16)
			copy(ip, data[offset:offset+16])
			ips = append(ips, ip)
		}

		offset += rdlength
	}

	return ips, nil
}

// splitHostname splits a hostname into DNS labels
func splitHostname(hostname string) []string {
	labels := make([]string, 0)
	current := ""
	for _, c := range hostname {
		if c == '.' {
			if current != "" {
				labels = append(labels, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		labels = append(labels, current)
	}
	return labels
}
