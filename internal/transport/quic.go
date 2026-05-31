package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// QUICTransport implements Transport using the QUIC protocol (via quic-go)
//
// QUIC advantages over TCP:
// - Built-in multiplexing with no head-of-line blocking
// - 0-RTT connection establishment (faster reconnects)
// - Better performance on lossy/mobile networks
// - Connection migration (survives IP changes)
// - UDP-based (avoids TCP-specific DPI signatures)
//
// Note: UDP may be blocked during severe filtering.
// Use with TCP fallback via multi-path failover.
type QUICTransport struct {
	cfg      *config.Config
	log      *logger.Logger
	listener *quic.Listener
	mu       sync.Mutex
}

// NewQUIC creates a new QUIC transport
func NewQUIC(cfg *config.Config, log *logger.Logger) *QUICTransport {
	return &QUICTransport{
		cfg: cfg,
		log: log,
	}
}

// Name returns the transport name
func (q *QUICTransport) Name() string {
	return "quic"
}

// Dial connects to the server using QUIC
func (q *QUICTransport) Dial() (net.Conn, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"ipshadowt-quic"},
	}

	quicConf := &quic.Config{
		MaxIdleTimeout:  60 * time.Second,
		KeepAlivePeriod: 15 * time.Second,
		Allow0RTT:       true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, q.cfg.RemoteAddr, tlsConf, quicConf)
	if err != nil {
		return nil, fmt.Errorf("QUIC dial to %s failed: %w", q.cfg.RemoteAddr, err)
	}

	// Open a single bidirectional stream for the mux layer
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		conn.CloseWithError(1, "stream open failed")
		return nil, fmt.Errorf("QUIC stream open failed: %w", err)
	}

	q.log.Debug("QUIC connected to %s", q.cfg.RemoteAddr)
	return &quicStreamConn{
		stream: stream,
		conn:   conn,
		local:  conn.LocalAddr(),
		remote: conn.RemoteAddr(),
	}, nil
}

// Listen starts accepting QUIC connections
func (q *QUICTransport) Listen() (net.Listener, error) {
	tlsConf := &tls.Config{
		NextProtos: []string{"ipshadowt-quic"},
	}

	// Load or generate TLS certificate
	if q.cfg.TLSCert != "" && q.cfg.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(q.cfg.TLSCert, q.cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS cert: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	} else {
		cert, err := generateQUICSelfSignedCert()
		if err != nil {
			return nil, fmt.Errorf("failed to generate TLS cert for QUIC: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	}

	quicConf := &quic.Config{
		MaxIdleTimeout:  60 * time.Second,
		KeepAlivePeriod: 15 * time.Second,
		Allow0RTT:       true,
	}

	listener, err := quic.ListenAddr(q.cfg.BindAddr, tlsConf, quicConf)
	if err != nil {
		return nil, fmt.Errorf("QUIC listen on %s failed: %w", q.cfg.BindAddr, err)
	}

	q.mu.Lock()
	q.listener = listener
	q.mu.Unlock()

	q.log.Info("QUIC transport listening on %s (UDP)", q.cfg.BindAddr)

	return &quicNetListener{
		listener: listener,
		log:      q.log,
		done:     make(chan struct{}),
	}, nil
}

// Close shuts down the transport
func (q *QUICTransport) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.listener != nil {
		return q.listener.Close()
	}
	return nil
}

// quicNetListener wraps quic.Listener as net.Listener
type quicNetListener struct {
	listener *quic.Listener
	log      *logger.Logger
	done     chan struct{}
}

func (l *quicNetListener) Accept() (net.Conn, error) {
	ctx := context.Background()
	conn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}

	// Accept a stream from the client
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		conn.CloseWithError(1, "stream accept failed")
		return nil, fmt.Errorf("QUIC accept stream failed: %w", err)
	}

	l.log.Debug("QUIC connection accepted from %s", conn.RemoteAddr())
	return &quicStreamConn{
		stream: stream,
		conn:   conn,
		local:  conn.LocalAddr(),
		remote: conn.RemoteAddr(),
	}, nil
}

func (l *quicNetListener) Close() error {
	close(l.done)
	return l.listener.Close()
}

func (l *quicNetListener) Addr() net.Addr {
	return l.listener.Addr()
}

// quicStreamConn wraps a QUIC stream as net.Conn
type quicStreamConn struct {
	stream *quic.Stream
	conn   *quic.Conn
	local  net.Addr
	remote net.Addr
}

func (c *quicStreamConn) Read(p []byte) (int, error) {
	return c.stream.Read(p)
}

func (c *quicStreamConn) Write(p []byte) (int, error) {
	return c.stream.Write(p)
}

func (c *quicStreamConn) Close() error {
	c.stream.Close()
	c.conn.CloseWithError(0, "closed")
	return nil
}

func (c *quicStreamConn) LocalAddr() net.Addr  { return c.local }
func (c *quicStreamConn) RemoteAddr() net.Addr { return c.remote }

func (c *quicStreamConn) SetDeadline(t time.Time) error {
	c.stream.SetDeadline(t)
	return nil
}

func (c *quicStreamConn) SetReadDeadline(t time.Time) error {
	c.stream.SetReadDeadline(t)
	return nil
}

func (c *quicStreamConn) SetWriteDeadline(t time.Time) error {
	c.stream.SetWriteDeadline(t)
	return nil
}

// generateQUICSelfSignedCert creates a self-signed TLS certificate for QUIC
func generateQUICSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate key: %w", err)
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"iPShadowT QUIC"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}
