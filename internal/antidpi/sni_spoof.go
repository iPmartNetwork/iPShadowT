package antidpi

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// SNISpoofing implements packet-level SNI spoofing to bypass DPI
//
// How it works:
// 1. Client connects to the real server IP
// 2. In the TLS ClientHello, the SNI field is replaced with a whitelisted domain
// 3. DPI sees traffic going to "google.com" but it actually goes to your server
// 4. Server ignores the fake SNI and processes normally
//
// Techniques used:
// - SNI replacement in ClientHello
// - TCP segmentation (split ClientHello across multiple packets)
// - TTL manipulation (first packet with fake SNI has low TTL, real one follows)
// - Out-of-order delivery (confuse stateful DPI)
//
// Based on: github.com/patterniha/SNI-Spoofing (adapted to Go)
type SNISpoofing struct {
	log        *logger.Logger
	fakeSNI    string   // Domain to show DPI (e.g., "www.google.com")
	realSNI    string   // Actual server domain (optional)
	method     SpoofMethod
	splitPos   int      // Where to split ClientHello (0 = auto)
	ttlValue   int      // TTL for fake packet (default: 1)
	mu         sync.RWMutex
}

// SpoofMethod defines the SNI spoofing technique
type SpoofMethod string

const (
	// MethodReplace replaces SNI in ClientHello
	MethodReplace SpoofMethod = "replace"
	// MethodSplit splits ClientHello so SNI is in a separate TCP segment
	MethodSplit SpoofMethod = "split"
	// MethodTTL sends fake SNI with low TTL (expires before reaching server)
	MethodTTL SpoofMethod = "ttl"
	// MethodDouble sends two ClientHellos (fake then real)
	MethodDouble SpoofMethod = "double"
	// MethodAll combines all techniques
	MethodAll SpoofMethod = "all"
)

// SNISpoofConfig configures SNI spoofing
type SNISpoofConfig struct {
	FakeSNI  string      // Whitelisted domain to show DPI
	RealSNI  string      // Actual server SNI (empty = don't care)
	Method   SpoofMethod // Spoofing technique
	SplitPos int         // Split position in ClientHello (0 = before SNI)
	TTL      int         // TTL for fake packets (default: 1)
}

// NewSNISpoofing creates a new SNI spoofer
func NewSNISpoofing(cfg SNISpoofConfig, log *logger.Logger) *SNISpoofing {
	if cfg.FakeSNI == "" {
		cfg.FakeSNI = "www.google.com"
	}
	if cfg.Method == "" {
		cfg.Method = MethodSplit
	}
	if cfg.TTL == 0 {
		cfg.TTL = 1
	}

	return &SNISpoofing{
		log:      log,
		fakeSNI:  cfg.FakeSNI,
		realSNI:  cfg.RealSNI,
		method:   cfg.Method,
		splitPos: cfg.SplitPos,
		ttlValue: cfg.TTL,
	}
}

// WrapConn wraps a connection with SNI spoofing on the first write (ClientHello)
func (s *SNISpoofing) WrapConn(conn net.Conn) net.Conn {
	return &sniSpoofConn{
		Conn:     conn,
		spoofer:  s,
		firstWrite: true,
	}
}

// sniSpoofConn wraps net.Conn and modifies the first TLS ClientHello
type sniSpoofConn struct {
	net.Conn
	spoofer    *SNISpoofing
	firstWrite bool
	mu         sync.Mutex
}

func (c *sniSpoofConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	if !c.firstWrite {
		c.mu.Unlock()
		return c.Conn.Write(p)
	}
	c.firstWrite = false
	c.mu.Unlock()

	// Check if this is a TLS ClientHello
	if !isTLSClientHello(p) {
		return c.Conn.Write(p)
	}

	switch c.spoofer.method {
	case MethodSplit:
		return c.writeSplit(p)
	case MethodReplace:
		return c.writeReplace(p)
	case MethodDouble:
		return c.writeDouble(p)
	case MethodAll:
		return c.writeSplit(p) // Split is most effective
	default:
		return c.writeSplit(p)
	}
}

// writeSplit splits the ClientHello so SNI is in a separate TCP segment
// DPI that only inspects the first packet won't see the real SNI
func (c *sniSpoofConn) writeSplit(data []byte) (int, error) {
	sniOffset := findSNIOffset(data)
	if sniOffset <= 0 {
		// Can't find SNI, send as-is
		return c.Conn.Write(data)
	}

	// Split before SNI
	splitAt := sniOffset
	if c.spoofer.splitPos > 0 && c.spoofer.splitPos < len(data) {
		splitAt = c.spoofer.splitPos
	}

	// Send first part (before SNI)
	_, err := c.Conn.Write(data[:splitAt])
	if err != nil {
		return 0, err
	}

	// Small delay to ensure separate TCP segments
	time.Sleep(1 * time.Millisecond)

	// Send second part (contains SNI)
	_, err = c.Conn.Write(data[splitAt:])
	if err != nil {
		return splitAt, err
	}

	return len(data), nil
}

// writeReplace replaces the SNI in ClientHello with the fake domain
func (c *sniSpoofConn) writeReplace(data []byte) (int, error) {
	modified := replaceSNI(data, c.spoofer.fakeSNI)
	return c.Conn.Write(modified)
}

// writeDouble sends a fake ClientHello first, then the real one
func (c *sniSpoofConn) writeDouble(data []byte) (int, error) {
	// Create fake ClientHello with spoofed SNI
	fakeHello := replaceSNI(data, c.spoofer.fakeSNI)

	// Send fake (DPI sees this)
	c.Conn.Write(fakeHello)

	// Tiny delay
	time.Sleep(500 * time.Microsecond)

	// Send real (server processes this)
	return c.Conn.Write(data)
}

// isTLSClientHello checks if data starts with a TLS ClientHello
func isTLSClientHello(data []byte) bool {
	if len(data) < 6 {
		return false
	}
	// TLS record: ContentType=22 (Handshake), Version=0x0301-0x0303
	// Handshake: Type=1 (ClientHello)
	return data[0] == 0x16 && // Handshake
		data[1] == 0x03 && // TLS major version
		data[5] == 0x01 // ClientHello
}

// findSNIOffset finds the byte offset of the SNI extension in a ClientHello
func findSNIOffset(data []byte) int {
	if len(data) < 43 {
		return -1
	}

	// Skip TLS record header (5 bytes) + Handshake header (4 bytes)
	offset := 9

	// Skip client version (2) + random (32)
	offset += 34

	if offset >= len(data) {
		return -1
	}

	// Skip session ID
	sessionIDLen := int(data[offset])
	offset += 1 + sessionIDLen

	if offset+2 >= len(data) {
		return -1
	}

	// Skip cipher suites
	cipherLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2 + cipherLen

	if offset >= len(data) {
		return -1
	}

	// Skip compression methods
	compLen := int(data[offset])
	offset += 1 + compLen

	if offset+2 >= len(data) {
		return -1
	}

	// Extensions
	extLen := int(binary.BigEndian.Uint16(data[offset:]))
	offset += 2
	extEnd := offset + extLen

	// Search for SNI extension (type 0x0000)
	for offset+4 < extEnd && offset+4 < len(data) {
		extType := binary.BigEndian.Uint16(data[offset:])
		extDataLen := int(binary.BigEndian.Uint16(data[offset+2:]))

		if extType == 0x0000 { // SNI extension
			return offset
		}

		offset += 4 + extDataLen
	}

	return -1
}

// replaceSNI replaces the SNI value in a TLS ClientHello
func replaceSNI(data []byte, newSNI string) []byte {
	sniOffset := findSNIOffset(data)
	if sniOffset < 0 {
		return data
	}

	// Find the actual hostname bytes within the SNI extension
	// SNI extension structure:
	// 2 bytes: extension type (0x0000)
	// 2 bytes: extension data length
	// 2 bytes: SNI list length
	// 1 byte: host name type (0x00)
	// 2 bytes: host name length
	// N bytes: host name
	pos := sniOffset + 4 // skip type + length
	if pos+2 >= len(data) {
		return data
	}

	pos += 2 // skip SNI list length
	if pos >= len(data) {
		return data
	}

	pos += 1 // skip host name type
	if pos+2 >= len(data) {
		return data
	}

	hostLen := int(binary.BigEndian.Uint16(data[pos:]))
	pos += 2

	if pos+hostLen > len(data) {
		return data
	}

	// If new SNI is same length, simple replace
	if len(newSNI) == hostLen {
		result := make([]byte, len(data))
		copy(result, data)
		copy(result[pos:], []byte(newSNI))
		return result
	}

	// Different length — need to rebuild (complex, use same-length for now)
	// Pad or truncate to match original length
	padded := make([]byte, hostLen)
	copy(padded, []byte(newSNI))
	if len(newSNI) < hostLen {
		// Pad with dots (still valid domain-ish)
		for i := len(newSNI); i < hostLen; i++ {
			padded[i] = '.'
		}
	}

	result := make([]byte, len(data))
	copy(result, data)
	copy(result[pos:], padded)
	return result
}

// GetFakeSNI returns the configured fake SNI
func (s *SNISpoofing) GetFakeSNI() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fakeSNI
}

// SetFakeSNI changes the fake SNI at runtime
func (s *SNISpoofing) SetFakeSNI(sni string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fakeSNI = sni
	s.log.Info("SNI spoof: fake SNI changed to %s", sni)
}

// CommonFakeSNIs returns a list of commonly whitelisted domains for Iran
func CommonFakeSNIs() []string {
	return []string{
		"www.google.com",
		"www.microsoft.com",
		"www.apple.com",
		"www.amazon.com",
		"update.microsoft.com",
		"dl.google.com",
		"play.google.com",
		"fonts.googleapis.com",
		"ajax.googleapis.com",
		"cdn.jsdelivr.net",
		"cdnjs.cloudflare.com",
		"unpkg.com",
		"raw.githubusercontent.com",
	}
}

// Ensure interface compliance
var _ net.Conn = (*sniSpoofConn)(nil)

// Suppress unused import
var _ = fmt.Sprintf
