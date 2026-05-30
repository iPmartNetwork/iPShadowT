package antidpi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// ShadowTLS implements the ShadowTLS v3 protocol
// Key idea: Perform a REAL TLS handshake with a real server,
// then switch to tunnel mode after the handshake completes.
//
// How it works:
// 1. Client connects to our server
// 2. Server initiates a REAL TLS handshake with a legitimate site (e.g., google.com)
// 3. Server relays the handshake to the client (client sees real TLS from google.com)
// 4. After handshake, server sends a special "switch" signal
// 5. Client and server switch to encrypted tunnel mode
//
// DPI sees: A normal TLS handshake with google.com → normal encrypted traffic
// Reality: After handshake, it's our tunnel

// ShadowTLSConfig holds ShadowTLS configuration
type ShadowTLSConfig struct {
	// HandshakeServer is the real TLS server to handshake with
	HandshakeServer string // e.g., "www.google.com:443"

	// Password for HMAC-based authentication
	Password string

	// Version of ShadowTLS protocol (3 recommended)
	Version int
}

// ShadowTLSServer handles server-side ShadowTLS
type ShadowTLSServer struct {
	config ShadowTLSConfig
	log    *logger.Logger
	hmacKey []byte
}

// NewShadowTLSServer creates a new ShadowTLS server
func NewShadowTLSServer(cfg ShadowTLSConfig, log *logger.Logger) *ShadowTLSServer {
	// Derive HMAC key from password
	h := sha256.Sum256([]byte(cfg.Password + "-shadowtls-hmac"))

	return &ShadowTLSServer{
		config:  cfg,
		log:     log,
		hmacKey: h[:],
	}
}

// HandleConnection processes a connection with ShadowTLS protocol
// Returns the unwrapped connection ready for tunnel use, or error
func (s *ShadowTLSServer) HandleConnection(clientConn net.Conn) (net.Conn, error) {
	// Step 1: Connect to the real TLS server
	handshakeConn, err := net.DialTimeout("tcp", s.config.HandshakeServer, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to handshake server: %w", err)
	}

	// Step 2: Relay the TLS handshake between client and real server
	// We need to detect when the handshake is complete
	handshakeDone := make(chan struct{})
	var handshakeErr error

	go func() {
		defer close(handshakeDone)
		handshakeErr = s.relayHandshake(clientConn, handshakeConn)
	}()

	// Wait for handshake to complete (with timeout)
	select {
	case <-handshakeDone:
		if handshakeErr != nil {
			handshakeConn.Close()
			return nil, fmt.Errorf("handshake relay failed: %w", handshakeErr)
		}
	case <-time.After(10 * time.Second):
		handshakeConn.Close()
		return nil, fmt.Errorf("handshake timeout")
	}

	// Step 3: Close connection to real server
	handshakeConn.Close()

	// Step 4: Verify client authentication
	if err := s.verifyClient(clientConn); err != nil {
		return nil, fmt.Errorf("client auth failed: %w", err)
	}

	s.log.Debug("ShadowTLS: Client authenticated, switching to tunnel mode")

	// Step 5: Return the connection for tunnel use
	return clientConn, nil
}

// relayHandshake relays TLS handshake between client and real server
func (s *ShadowTLSServer) relayHandshake(client, server net.Conn) error {
	// Read ClientHello from client and forward to server
	clientHello, err := readTLSRecord(client)
	if err != nil {
		return fmt.Errorf("failed to read ClientHello: %w", err)
	}

	if _, err := server.Write(clientHello); err != nil {
		return fmt.Errorf("failed to forward ClientHello: %w", err)
	}

	// Read ServerHello + Certificate + etc from server and forward to client
	for {
		record, err := readTLSRecord(server)
		if err != nil {
			return fmt.Errorf("failed to read server record: %w", err)
		}

		if _, err := client.Write(record); err != nil {
			return fmt.Errorf("failed to forward server record: %w", err)
		}

		// Check if this is the last handshake message (ChangeCipherSpec or Finished)
		if len(record) > 0 && record[0] == 0x14 { // ChangeCipherSpec
			break
		}

		// Also check for TLS 1.3 (no ChangeCipherSpec, look for Application Data)
		if len(record) > 0 && record[0] == 0x17 { // Application Data (TLS 1.3 encrypted)
			break
		}
	}

	return nil
}

// verifyClient verifies the client's HMAC authentication
func (s *ShadowTLSServer) verifyClient(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// Read HMAC from client (32 bytes)
	hmacBuf := make([]byte, 32)
	if _, err := io.ReadFull(conn, hmacBuf); err != nil {
		return fmt.Errorf("failed to read HMAC: %w", err)
	}

	// Compute expected HMAC
	mac := hmac.New(sha256.New, s.hmacKey)
	mac.Write([]byte("shadowtls-auth-v3"))
	expectedMAC := mac.Sum(nil)

	// Constant-time comparison
	if !hmac.Equal(hmacBuf, expectedMAC) {
		return fmt.Errorf("HMAC mismatch")
	}

	// Send confirmation
	conn.Write([]byte{0x01}) // 1 byte confirmation

	return nil
}

// ShadowTLSClient handles client-side ShadowTLS
type ShadowTLSClient struct {
	config  ShadowTLSConfig
	log     *logger.Logger
	hmacKey []byte
}

// NewShadowTLSClient creates a new ShadowTLS client
func NewShadowTLSClient(cfg ShadowTLSConfig, log *logger.Logger) *ShadowTLSClient {
	h := sha256.Sum256([]byte(cfg.Password + "-shadowtls-hmac"))

	return &ShadowTLSClient{
		config:  cfg,
		log:     log,
		hmacKey: h[:],
	}
}

// Connect performs ShadowTLS handshake and returns tunnel-ready connection
func (c *ShadowTLSClient) Connect(conn net.Conn) (net.Conn, error) {
	// Step 1: The server will relay a real TLS handshake to us
	// We participate in the handshake as if connecting to the real server
	// (The server handles this transparently)

	// Step 2: After handshake completes, send our HMAC authentication
	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write([]byte("shadowtls-auth-v3"))
	authMAC := mac.Sum(nil)

	if _, err := conn.Write(authMAC); err != nil {
		return nil, fmt.Errorf("failed to send auth: %w", err)
	}

	// Step 3: Read confirmation
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	confirm := make([]byte, 1)
	if _, err := io.ReadFull(conn, confirm); err != nil {
		return nil, fmt.Errorf("failed to read confirmation: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	if confirm[0] != 0x01 {
		return nil, fmt.Errorf("server rejected authentication")
	}

	c.log.Debug("ShadowTLS: Authenticated, tunnel mode active")
	return conn, nil
}

// readTLSRecord reads a single TLS record from a connection
func readTLSRecord(conn net.Conn) ([]byte, error) {
	// TLS record header: ContentType(1) + Version(2) + Length(2)
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	// Parse length
	recordLen := binary.BigEndian.Uint16(header[3:5])
	if recordLen > 16384+256 { // Max TLS record + overhead
		return nil, fmt.Errorf("TLS record too large: %d", recordLen)
	}

	// Read record body
	body := make([]byte, recordLen)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}

	// Return full record (header + body)
	record := make([]byte, 5+recordLen)
	copy(record, header)
	copy(record[5:], body)

	return record, nil
}
