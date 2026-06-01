package antidpi

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"time"

	utls "github.com/refraction-networking/utls"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// RealityConfig holds REALITY protocol configuration
type RealityConfig struct {
	Dest        string
	ServerName  string
	PrivateKey  string
	ShortIDs    []string
	PublicKey   string
	ShortID     string
	Fingerprint string
}

// RealityServer handles REALITY protocol on the server side
type RealityServer struct {
	config     RealityConfig
	privateKey *ecdh.PrivateKey
	log        *logger.Logger
	replay     *ReplayProtection
}

// NewRealityServer creates a new REALITY server
func NewRealityServer(cfg RealityConfig, log *logger.Logger) (*RealityServer, error) {
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
		replay:     NewReplayProtection(2 * time.Minute),
	}, nil
}

func (rs *RealityServer) debug(format string, args ...interface{}) {
	if rs.log != nil {
		rs.log.Debug(format, args...)
	}
}

// HandleConnection processes an incoming connection with REALITY protocol.
func (rs *RealityServer) HandleConnection(conn net.Conn) (net.Conn, bool, error) {
	rawData, err := rs.readClientHelloRecord(conn)
	if err != nil {
		rs.debug("REALITY: Failed to read ClientHello: %v", err)
		rs.proxyToFallback(conn, rawData)
		return nil, false, nil
	}

	if !rs.verifyClientHello(rawData) {
		rs.debug("REALITY: Client verification failed, proxying to fallback")
		rs.proxyToFallback(conn, rawData)
		return nil, false, nil
	}

	if _, err := conn.Write([]byte(RealityAuthOK)); err != nil {
		conn.Close()
		return nil, false, fmt.Errorf("failed to send REALITY auth ack: %w", err)
	}

	rs.debug("REALITY: Client authenticated successfully")
	return conn, true, nil
}

func (rs *RealityServer) readClientHelloRecord(conn net.Conn) ([]byte, error) {
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return header, err
	}
	if header[0] != 0x16 {
		return header, fmt.Errorf("not a TLS handshake: type=%d", header[0])
	}

	recordLen := int(header[3])<<8 | int(header[4])
	if recordLen <= 0 || recordLen > 16384 {
		return header, fmt.Errorf("invalid record length: %d", recordLen)
	}

	record := make([]byte, recordLen)
	if _, err := io.ReadFull(conn, record); err != nil {
		return append(header, record...), err
	}

	return append(header, record...), nil
}

func (rs *RealityServer) verifyClientHello(rawData []byte) bool {
	sessionID, ephPub, err := parseClientHelloAuth(rawData)
	if err != nil {
		rs.debug("REALITY: parse ClientHello: %v", err)
		return false
	}

	if len(sessionID) != AuthTokenSize {
		return false
	}
	if !rs.replay.Check(sessionID) {
		rs.debug("REALITY: replay detected")
		return false
	}

	ok, _ := VerifyAuthToken(sessionID, ephPub, rs.privateKey, rs.config.ShortIDs, 120)
	return ok
}

func (rs *RealityServer) proxyToFallback(conn net.Conn, initialData []byte) {
	defer conn.Close()

	if rs.config.Dest == "" {
		return
	}

	fallbackConn, err := net.DialTimeout("tcp", rs.config.Dest, 5*time.Second)
	if err != nil {
		rs.debug("REALITY: Fallback connection failed: %v", err)
		return
	}
	defer fallbackConn.Close()

	if len(initialData) > 0 {
		if _, err := fallbackConn.Write(initialData); err != nil {
			return
		}
	}

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
	keyBytes, err := hex.DecodeString(cfg.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	publicKey, err := ecdh.X25519().NewPublicKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

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
	if err := rc.sendAuthenticatedClientHello(conn); err != nil {
		return nil, err
	}

	resp := make([]byte, len(RealityAuthOK))
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, fmt.Errorf("REALITY auth response failed: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	if string(resp) != RealityAuthOK {
		return nil, fmt.Errorf("REALITY authentication rejected by server")
	}

	rc.debug("REALITY: Connected (SNI: %s, fingerprint: %s)", rc.config.ServerName, rc.config.Fingerprint)
	return conn, nil
}

func (rc *RealityClient) debug(format string, args ...interface{}) {
	if rc.log != nil {
		rc.log.Debug(format, args...)
	}
}

func (rc *RealityClient) sendAuthenticatedClientHello(conn net.Conn) error {
	token, ephemeralKey, err := BuildAuthToken(rc.publicKey, rc.config.ShortID, 120)
	if err != nil {
		return fmt.Errorf("build auth token: %w", err)
	}

	spec, err := utls.UTLSIdToSpec(rc.utls.getClientHelloID())
	if err != nil {
		return err
	}
	injectRealityKeyShare(&spec, ephemeralKey.PublicKey().Bytes())

	tlsConfig := &utls.Config{
		ServerName:             rc.config.ServerName,
		InsecureSkipVerify:     true,
		SessionTicketsDisabled: true,
	}

	uconn := utls.UClient(conn, tlsConfig, utls.HelloCustom)
	if err := uconn.ApplyPreset(&spec); err != nil {
		return fmt.Errorf("apply ClientHello preset: %w", err)
	}
	if err := uconn.BuildHandshakeState(); err != nil {
		return fmt.Errorf("build handshake state: %w", err)
	}

	sessionID := SerializeAuthToken(token)
	uconn.HandshakeState.Hello.SessionId = sessionID
	patchHelloX25519KeyShare(uconn.HandshakeState.Hello, ephemeralKey.PublicKey().Bytes())

	if err := uconn.MarshalClientHello(); err != nil {
		return fmt.Errorf("marshal ClientHello: %w", err)
	}

	raw := uconn.HandshakeState.Hello.Raw
	if len(raw) == 0 {
		return fmt.Errorf("empty ClientHello")
	}
	record := wrapTLSHandshakeRecord(raw)

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetWriteDeadline(time.Time{})

	if _, err := conn.Write(record); err != nil {
		return fmt.Errorf("write ClientHello: %w", err)
	}
	return nil
}

func wrapTLSHandshakeRecord(handshake []byte) []byte {
	record := make([]byte, 5+len(handshake))
	record[0] = 0x16 // handshake
	record[1] = 0x03
	record[2] = 0x01
	record[3] = byte(len(handshake) >> 8)
	record[4] = byte(len(handshake))
	copy(record[5:], handshake)
	return record
}

func injectRealityKeyShare(spec *utls.ClientHelloSpec, ephPub []byte) {
	for _, ext := range spec.Extensions {
		ksExt, ok := ext.(*utls.KeyShareExtension)
		if !ok {
			continue
		}
		for i := range ksExt.KeyShares {
			if ksExt.KeyShares[i].Group == utls.X25519 {
				ksExt.KeyShares[i].Data = append([]byte(nil), ephPub...)
			}
		}
	}
}

func patchHelloX25519KeyShare(hello *utls.PubClientHelloMsg, ephPub []byte) {
	for i := range hello.KeyShares {
		if hello.KeyShares[i].Group == utls.X25519 {
			hello.KeyShares[i].Data = append([]byte(nil), ephPub...)
		}
	}
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
