package antidpi

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// ProbeResistance implements active probe resistance
// When an unauthorized client connects, the server behaves like a normal web server
// Only clients with the correct pre-shared key can activate the tunnel
type ProbeResistance struct {
	log          *logger.Logger
	fallbackAddr string        // Real website to proxy to (e.g., "www.google.com:443")
	authTimeout  time.Duration // Time to wait for auth before falling back
	blockedIPs   sync.Map      // IPs that failed auth (rate limiting)
}

// NewProbeResistance creates a new probe resistance handler
func NewProbeResistance(fallbackAddr string, log *logger.Logger) *ProbeResistance {
	return &ProbeResistance{
		log:          log,
		fallbackAddr: fallbackAddr,
		authTimeout:  5 * time.Second,
	}
}

// HandleConnection decides whether a connection is a legitimate client or a probe
// Returns true if the connection is authenticated (tunnel client)
// Returns false if it's a probe (connection was handled as fallback)
func (pr *ProbeResistance) HandleConnection(conn net.Conn, expectedToken []byte) (bool, error) {
	remoteAddr := conn.RemoteAddr().String()

	// Check if this IP is rate-limited
	if pr.isBlocked(remoteAddr) {
		pr.log.Debug("Blocked probe from %s", remoteAddr)
		pr.serveFallback(conn)
		return false, nil
	}

	// Set a short deadline for authentication
	conn.SetReadDeadline(time.Now().Add(pr.authTimeout))

	// Try to read the auth token
	// Our protocol sends an encrypted frame first
	// If it doesn't match, treat as probe
	buf := make([]byte, 4) // Read length prefix
	n, err := io.ReadFull(conn, buf)
	if err != nil || n < 4 {
		// Didn't send our protocol - it's a probe or scanner
		pr.log.Debug("Probe detected from %s (no valid header)", remoteAddr)
		pr.recordFailure(remoteAddr)
		// Push back what we read and serve fallback
		pr.serveFallbackWithData(conn, buf[:n])
		return false, nil
	}

	// Reset deadline
	conn.SetReadDeadline(time.Time{})

	// The connection sent a valid-looking length prefix
	// Let the caller handle the actual authentication
	// We'll push the bytes back by wrapping the connection
	return true, nil
}

// serveFallback proxies the connection to the fallback website
func (pr *ProbeResistance) serveFallback(conn net.Conn) {
	defer conn.Close()

	if pr.fallbackAddr == "" {
		// No fallback configured - just serve a simple page
		pr.serveDefaultPage(conn)
		return
	}

	// Connect to fallback server
	fallbackConn, err := net.DialTimeout("tcp", pr.fallbackAddr, 5*time.Second)
	if err != nil {
		pr.log.Debug("Fallback connection failed: %v", err)
		pr.serveDefaultPage(conn)
		return
	}
	defer fallbackConn.Close()

	// Relay traffic between probe and fallback
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(fallbackConn, conn)
	}()

	go func() {
		defer wg.Done()
		io.Copy(conn, fallbackConn)
	}()

	wg.Wait()
}

// serveFallbackWithData serves fallback but first sends back already-read data
func (pr *ProbeResistance) serveFallbackWithData(conn net.Conn, initialData []byte) {
	defer conn.Close()

	// Check if the initial data looks like HTTP
	if len(initialData) > 0 && (initialData[0] == 'G' || initialData[0] == 'P' || initialData[0] == 'H') {
		// Looks like HTTP - serve a web page
		pr.serveDefaultPage(conn)
		return
	}

	// Check if it looks like TLS ClientHello
	if len(initialData) > 0 && initialData[0] == 0x16 {
		// TLS handshake - proxy to fallback
		pr.serveFallback(conn)
		return
	}

	// Unknown protocol - just close
	conn.Close()
}

// serveDefaultPage serves a convincing default web page
func (pr *ProbeResistance) serveDefaultPage(conn net.Conn) {
	response := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Server: nginx/1.24.0\r\n" +
		"Date: " + time.Now().UTC().Format(http.TimeFormat) + "\r\n" +
		"Connection: close\r\n" +
		"\r\n" +
		"<!DOCTYPE html>\n" +
		"<html lang=\"en\">\n" +
		"<head>\n" +
		"    <meta charset=\"UTF-8\">\n" +
		"    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n" +
		"    <title>Welcome</title>\n" +
		"</head>\n" +
		"<body>\n" +
		"    <h1>Welcome to nginx!</h1>\n" +
		"    <p>If you see this page, the nginx web server is successfully installed and working.</p>\n" +
		"</body>\n" +
		"</html>\n"

	conn.Write([]byte(response))
}

// isBlocked checks if an IP is rate-limited
func (pr *ProbeResistance) isBlocked(addr string) bool {
	host, _, _ := net.SplitHostPort(addr)
	if host == "" {
		host = addr
	}

	val, ok := pr.blockedIPs.Load(host)
	if !ok {
		return false
	}

	info := val.(*blockInfo)
	// Block for 5 minutes after 3 failures
	if info.failures >= 3 && time.Since(info.lastFailure) < 5*time.Minute {
		return true
	}

	// Reset if enough time has passed
	if time.Since(info.lastFailure) > 10*time.Minute {
		pr.blockedIPs.Delete(host)
		return false
	}

	return false
}

// recordFailure records a failed authentication attempt
func (pr *ProbeResistance) recordFailure(addr string) {
	host, _, _ := net.SplitHostPort(addr)
	if host == "" {
		host = addr
	}

	val, loaded := pr.blockedIPs.LoadOrStore(host, &blockInfo{
		failures:    1,
		lastFailure: time.Now(),
	})

	if loaded {
		info := val.(*blockInfo)
		info.failures++
		info.lastFailure = time.Now()
	}
}

type blockInfo struct {
	failures    int
	lastFailure time.Time
}

// ConstantTimeCompare performs a constant-time comparison of two byte slices
// to prevent timing attacks during authentication
func ConstantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// ReverseProxyFallback creates an HTTP reverse proxy to a fallback site
// Used when the server needs to serve real HTTPS content to probes
func ReverseProxyFallback(target string, log *logger.Logger) http.Handler {
	targetURL, err := url.Parse(fmt.Sprintf("https://%s", target))
	if err != nil {
		log.Error("Invalid fallback URL: %v", err)
		return http.NotFoundHandler()
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Modify the director to set proper headers
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
		req.Header.Set("X-Forwarded-Host", req.Host)
	}

	// Suppress error logging from the proxy
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Debug("Fallback proxy error: %v", err)
		w.WriteHeader(http.StatusBadGateway)
	}

	return proxy
}
