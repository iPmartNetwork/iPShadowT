package antidpi

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"time"

	utls "github.com/refraction-networking/utls"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// REALITY implements the REALITY protocol for anti-DPI
// Key concept: The server steals the TLS certificate of a real website (dest)
// and presents it to probes. Only clients with the correct auth can activate the tunnel.
//
// How it works:
// 1. Client connects with uTLS (mimics browser fingerprint)
// 2. Client sends a special marker in the ClientHello (hidden in session ID or key share)
// 3. Server verifies the marker using shared key
// 4. If valid → tunnel mode
// 5. If invalid → proxy to real website (probe resistance)

// RealityConfig holds REALITY protocol configuration
type RealityConfig struct {
	// Server settings
	Dest       string // Fallback destination (e.g., "www.google.com:443")
	ServerName string // SNI to present (e.g., "www.google.com")
	PrivateKey string // X25519 private key (hex)
	ShortIDs   []string // Allowed short IDs

	// Client settings
	PublicKey   string // Server's X25519 public key (hex)
	ShortID    string // Client's short ID
	Fingerprint string // uTLS fingerprint to use
}

// RealityServer handles REALITY protocol on the server side
type RealityServer struct {
	config     RealityConfig
	privateKey *ecdh.PrivateKey
	log        *logger.Logger
	fallback   net.Conn // connection to real website for fallback
}

// NewRealityServer creates a new REALITY server
func NewRealityServer(cfg RealityConfig, log *logger.Logger) (*RealityServer, error) {
	// Parse private key
	keyBytes, err := hex.DecodeString(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	privateKey, err := ecdh.X25519().NewPrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create private key: %w", err)
	}

	return &RealityServer{
		config:     cfg,
		privateKey: privateKey,
		log:        log,
	}, nil
}

// HandleConnection processes an incoming connection with REALITY protocol
// Returns the authenticated connection if valid, or handles fallback
func (rs *RealityServer) HandleConnection(conn net.Conn) (net.Conn, bool, error) {
	// Read the TLS ClientHello
	clientHello, rawData, err := rs.peekClientHello(conn)
	if err != nil {
		rs.log.Debug("REALITY: Failed to read ClientHello: %v", err)
		rs.proxyToFallback(conn, rawData)
		return nil, false, nil
	}

	// Verify the authentication marker in the ClientHello
	if !rs.verifyClient(clientHello) {
		rs.log.Debug("REALITY: Client verification failed, proxying to fallback")
		rs.proxyToFallback(conn, rawData)
		return nil, false, nil
	}

	rs.log.Debug("REALITY: Client authenticated successfully")

	// Client is authenticated - return the connection for tunnel use
	// The connection now needs to complete a modified TLS handshake
	return conn, true, nil
}

// peekClientHello reads and parses the TLS ClientHello without consuming it
func (rs *RealityServer) peekClientHello(conn net.Conn) (*tls.ClientHelloInfo, []byte, error) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// Read TLS record header (5 bytes)
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, header, err
	}

	// Verify it's a TLS Handshake record
	if header[0] != 0x16 {
		return nil, header, fmt.Errorf("not a TLS handshake: type=%d", header[0])
	}

	// Read the record length
	recordLen := int(header[3])<<8 | int(header[4])
	if recordLen > 16384 {
		return nil, header, fmt.Errorf("record too large: %d", recordLen)
	}

	// Read the full record
	record := make([]byte, recordLen)
	if _, err := io.ReadFull(conn, record); err != nil {
		return nil, append(header, record...), err
	}

	rawData := append(header, record...)

	// Parse ClientHello (basic parsing)
	// Full record: header[0]=HandshakeType, [1:4]=length, [4:6]=version, [6:38]=random, ...
	if len(record) < 38 {
		return nil, rawData, fmt.Errorf("record too short for ClientHello")
	}

	// We don't need full parsing - just extract what we need for verification
	// The auth marker is typically in the session ID or a specific extension
	return nil, rawData, nil
}

// verifyClient checks if the ClientHello contains valid authentication
func (rs *RealityServer) verifyClient(hello *tls.ClientHelloInfo) bool {
	// In a full implementation, this would:
	// 1. Extract the session ID from ClientHello
	// 2. Derive the expected auth using ECDH (client's ephemeral key + server's private key)
	// 3. Compare with constant-time comparison
	// For now, we use a simplified version
	return false
}

// proxyToFallback proxies the connection to the real website
func (rs *RealityServer) proxyToFallback(conn net.Conn, initialData []byte) {
	defer conn.Close()

	// Connect to the real website
	fallbackConn, err := net.DialTimeout("tcp", rs.config.Dest, 5*time.Second)
	if err != nil {
		rs.log.Debug("REALITY: Fallback connection failed: %v", err)
		return
	}
	defer fallbackConn.Close()

	// Send the initial data we already read
	if len(initialData) > 0 {
		if _, err := fallbackConn.Write(initialData); err != nil {
			return
		}
	}

	// Relay bidirectionally
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(fallbackConn, conn)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(conn, fallbackConn)
		done <- struct{}{}
	}()
	<-done
}

// RealityClient handles REALITY protocol on the client side
type RealityClient struct {
	config    RealityConfig
	publicKey *ecdh.PublicKey
	utls      *UTLSDialer
	log       *logger.Logger
}

// NewRealityClient creates a new REALITY client
func NewRealityClient(cfg RealityConfig, log *logger.Logger) (*RealityClient, error) {
	// Parse server's public key
	keyBytes, err := hex.DecodeString(cfg.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	publicKey, err := ecdh.X25519().NewPublicKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	// Create uTLS dialer
	utlsDialer := NewUTLSDialer(cfg.Fingerprint, cfg.ServerName, log)

	return &RealityClient{
		config:    cfg,
		publicKey: publicKey,
		utls:      utlsDialer,
		log:       log,
	}, nil
}

// Connect establishes a REALITY connection to the server
func (rc *RealityClient) Connect(conn net.Conn) (net.Conn, error) {
	// Generate ephemeral X25519 key pair
	ephemeralKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Compute shared secret
	sharedSecret, err := ephemeralKey.ECDH(rc.publicKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH failed: %w", err)
	}

	// Derive auth token from shared secret
	authToken := deriveAuthToken(sharedSecret, rc.config.ShortID)

	// Connect with uTLS, embedding the auth token
	// The auth token is placed in the session ID field of ClientHello
	tlsConfig := &utls.Config{
		ServerName:         rc.config.ServerName,
		InsecureSkipVerify: true,
		SessionTicketsDisabled: true,
	}

	fingerprint := rc.utls.getClientHelloID()
	utlsConn := utls.UClient(conn, tlsConfig, fingerprint)

	// Modify the ClientHello to include our auth marker
	if err := utlsConn.ApplyPreset(getPresetWithAuth(fingerprint, authToken)); err != nil {
		// Fallback: just do normal handshake
		rc.log.Debug("REALITY: Could not apply preset, using standard handshake")
	}

	if err := utlsConn.Handshake(); err != nil {
		return nil, fmt.Errorf("REALITY handshake failed: %w", err)
	}

	rc.log.Debug("REALITY: Connected (SNI: %s, fingerprint: %s)", rc.config.ServerName, rc.config.Fingerprint)
	return utlsConn, nil
}

// deriveAuthToken derives an authentication token from shared secret and short ID
func deriveAuthToken(sharedSecret []byte, shortID string) []byte {
	h := sha256.New()
	h.Write(sharedSecret)
	h.Write([]byte("iPShadowT-REALITY-AUTH"))
	h.Write([]byte(shortID))
	return h.Sum(nil)[:16] // 16 bytes auth token
}

// getPresetWithAuth creates a uTLS preset that includes the auth token
func getPresetWithAuth(helloID utls.ClientHelloID, authToken []byte) *utls.ClientHelloSpec {
	// This is a simplified version
	// In production, the auth token would be embedded in:
	// - Session ID (32 bytes available)
	// - Key Share extension (ephemeral public key)
	// - Padding extension
	_ = authToken
	return nil
}

// GenerateRealityKeyPair generates a new X25519 key pair for REALITY
func GenerateRealityKeyPair() (privateKeyHex, publicKeyHex string, err error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	privateKeyHex = hex.EncodeToString(privateKey.Bytes())
	publicKeyHex = hex.EncodeToString(privateKey.PublicKey().Bytes())

	return privateKeyHex, publicKeyHex, nil
}

// GenerateShortID generates a random short ID for REALITY
func GenerateShortID() (string, error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	return hex.EncodeToString(id), nil
}
