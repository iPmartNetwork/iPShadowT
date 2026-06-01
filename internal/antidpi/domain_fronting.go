package antidpi

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// DomainFronting implements client-side domain fronting
//
// How it works (inspired by patterniha/MITM-DomainFronting):
// 1. TLS connection is made to a CDN edge with a whitelisted SNI (e.g., "allowed.com")
// 2. Inside the encrypted TLS, the HTTP Host header points to the real destination
// 3. CDN routes based on Host header, not SNI
// 4. DPI only sees SNI="allowed.com" (whitelisted), can't see Host inside TLS
//
// Supported CDNs:
// - Cloudflare: many domains share same edge IPs
// - Google: *.google.com, *.googleapis.com
// - Fastly: shared edge infrastructure
// - Akamai: large shared pool
//
// No server-side changes needed — works with any CDN-fronted service
type DomainFronting struct {
	log       *logger.Logger
	cfg       DomainFrontingConfig
	client    *http.Client
	transport *http.Transport
	mu        sync.RWMutex
}

// DomainFrontingConfig configures domain fronting
type DomainFrontingConfig struct {
	// FrontDomain is the domain shown in SNI (must be on same CDN as real host)
	FrontDomain string
	// RealHost is the actual Host header sent inside TLS
	RealHost string
	// CDNAddr is the CDN edge IP:port to connect to (optional, resolved from FrontDomain)
	CDNAddr string
	// Path is the URL path for the tunnel endpoint
	Path string
	// Headers are additional HTTP headers to send
	Headers map[string]string
	// AllowInsecure skips TLS verification (for testing)
	AllowInsecure bool
}

// NewDomainFronting creates a new domain fronting connector
func NewDomainFronting(cfg DomainFrontingConfig, log *logger.Logger) *DomainFronting {
	if cfg.Path == "" {
		cfg.Path = "/tunnel"
	}
	if cfg.Headers == nil {
		cfg.Headers = make(map[string]string)
	}

	tlsConfig := &tls.Config{
		ServerName:         cfg.FrontDomain, // SNI = front domain (DPI sees this)
		InsecureSkipVerify: cfg.AllowInsecure,
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false,
	}

	// If CDN address is specified, dial directly to it
	if cfg.CDNAddr != "" {
		transport.DialTLS = func(network, addr string) (net.Conn, error) {
			// Connect to CDN edge directly
			conn, err := net.DialTimeout("tcp", cfg.CDNAddr, 10*time.Second)
			if err != nil {
				return nil, err
			}
			// TLS handshake with front domain as SNI
			tlsConn := tls.Client(conn, tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				conn.Close()
				return nil, err
			}
			return tlsConn, nil
		}
	}

	return &DomainFronting{
		log:       log,
		cfg:       cfg,
		transport: transport,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// Connect establishes a fronted connection and returns a bidirectional stream
// The connection looks like HTTPS to "FrontDomain" but actually reaches "RealHost"
func (df *DomainFronting) Connect() (net.Conn, error) {
	// Build the URL with front domain (for TLS/SNI)
	url := fmt.Sprintf("https://%s%s", df.cfg.FrontDomain, df.cfg.Path)

	// Create HTTP request with real Host header
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set the REAL host (CDN routes based on this, inside TLS)
	req.Host = df.cfg.RealHost

	// Set headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	for k, v := range df.cfg.Headers {
		req.Header.Set(k, v)
	}

	df.log.Debug("Domain fronting: SNI=%s, Host=%s, Path=%s",
		df.cfg.FrontDomain, df.cfg.RealHost, df.cfg.Path)

	// Perform the request
	resp, err := df.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fronted request failed: %w", err)
	}

	// Check for WebSocket upgrade
	if resp.StatusCode == 101 {
		// WebSocket upgrade successful — use the connection as a tunnel
		conn, ok := resp.Body.(net.Conn)
		if ok {
			return conn, nil
		}
		// Fallback: wrap response body as a connection
		return &frontedConn{
			reader: resp.Body,
			writer: nil, // Write not supported in this mode
			local:  &net.TCPAddr{},
			remote: &net.TCPAddr{},
		}, nil
	}

	// For non-WebSocket, use HTTP streaming
	if resp.StatusCode == 200 {
		return &frontedConn{
			reader: resp.Body,
			writer: nil,
			local:  &net.TCPAddr{},
			remote: &net.TCPAddr{},
		}, nil
	}

	resp.Body.Close()
	return nil, fmt.Errorf("fronted request returned status %d", resp.StatusCode)
}

// ConnectRaw establishes a raw TLS connection with domain fronting
// Returns a net.Conn that has TLS with SNI=FrontDomain
// Caller can then send any protocol inside
func (df *DomainFronting) ConnectRaw() (net.Conn, error) {
	addr := df.cfg.CDNAddr
	if addr == "" {
		addr = df.cfg.FrontDomain + ":443"
	}

	// TCP connect
	tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("TCP dial to CDN: %w", err)
	}

	// TLS handshake with front domain as SNI
	tlsConfig := &tls.Config{
		ServerName:         df.cfg.FrontDomain,
		InsecureSkipVerify: df.cfg.AllowInsecure,
	}

	tlsConn := tls.Client(tcpConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("TLS handshake: %w", err)
	}

	df.log.Debug("Domain fronting raw: connected via %s (SNI=%s)", addr, df.cfg.FrontDomain)
	return tlsConn, nil
}

// frontedConn wraps an HTTP response as a net.Conn (read-only in basic mode)
type frontedConn struct {
	reader io.ReadCloser
	writer io.Writer
	local  net.Addr
	remote net.Addr
}

func (c *frontedConn) Read(p []byte) (int, error) {
	if c.reader == nil {
		return 0, io.EOF
	}
	return c.reader.Read(p)
}

func (c *frontedConn) Write(p []byte) (int, error) {
	if c.writer == nil {
		return 0, fmt.Errorf("write not supported in this fronting mode")
	}
	return c.writer.Write(p)
}

func (c *frontedConn) Close() error {
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}

func (c *frontedConn) LocalAddr() net.Addr                { return c.local }
func (c *frontedConn) RemoteAddr() net.Addr               { return c.remote }
func (c *frontedConn) SetDeadline(t time.Time) error      { return nil }
func (c *frontedConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *frontedConn) SetWriteDeadline(t time.Time) error { return nil }

// KnownFrontableDomains returns domains known to work for fronting on major CDNs
func KnownFrontableDomains() map[string][]string {
	return map[string][]string{
		"cloudflare": {
			"cdnjs.cloudflare.com",
			"ajax.cloudflare.com",
			"www.cloudflare.com",
		},
		"google": {
			"www.google.com",
			"fonts.googleapis.com",
			"ajax.googleapis.com",
			"dl.google.com",
			"play.google.com",
			"maps.googleapis.com",
		},
		"fastly": {
			"cdn.jsdelivr.net",
			"unpkg.com",
		},
		"amazon": {
			"d1.awsstatic.com",
			"images-na.ssl-images-amazon.com",
		},
	}
}

// FindFrontDomain finds a suitable front domain for a given CDN provider
func FindFrontDomain(provider string) string {
	domains := KnownFrontableDomains()
	if list, ok := domains[provider]; ok && len(list) > 0 {
		return list[0]
	}
	return "www.google.com" // Default fallback
}
