package transport

import (
	"fmt"
	"net"
	"time"

	"github.com/iPmart/iPShadowT/internal/antidpi"
	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// ShadowTLSTransport implements Transport using ShadowTLS v3
// Advantages over REALITY:
// - No certificate needed on the server
// - Uses a real TLS handshake from a real server
// - Very hard to distinguish from normal HTTPS
type ShadowTLSTransport struct {
	cfg      *config.Config
	log      *logger.Logger
	listener net.Listener
	server   *antidpi.ShadowTLSServer
	client   *antidpi.ShadowTLSClient
}

// NewShadowTLS creates a new ShadowTLS transport
func NewShadowTLS(cfg *config.Config, log *logger.Logger) *ShadowTLSTransport {
	stlsCfg := antidpi.ShadowTLSConfig{
		HandshakeServer: cfg.Reality.Dest, // Reuse the dest field
		Password:        cfg.Password,
		Version:         3,
	}

	t := &ShadowTLSTransport{
		cfg: cfg,
		log: log,
	}

	if cfg.Mode == "server" {
		t.server = antidpi.NewShadowTLSServer(stlsCfg, log)
	} else {
		t.client = antidpi.NewShadowTLSClient(stlsCfg, log)
	}

	return t
}

// Name returns the transport name
func (t *ShadowTLSTransport) Name() string {
	return "shadowtls"
}

// Dial connects using ShadowTLS
func (t *ShadowTLSTransport) Dial() (net.Conn, error) {
	if t.client == nil {
		return nil, fmt.Errorf("ShadowTLS client not initialized")
	}

	// TCP connection
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: time.Duration(t.cfg.Performance.KeepAlive) * time.Second,
	}

	tcpConn, err := dialer.Dial("tcp", t.cfg.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("TCP dial failed: %w", err)
	}

	// ShadowTLS handshake
	tunnelConn, err := t.client.Connect(tcpConn)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("ShadowTLS connect failed: %w", err)
	}

	t.log.Debug("ShadowTLS connected to %s", t.cfg.RemoteAddr)
	return tunnelConn, nil
}

// Listen starts accepting ShadowTLS connections
func (t *ShadowTLSTransport) Listen() (net.Listener, error) {
	addr := t.cfg.BindAddr

	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s failed: %w", addr, err)
	}

	stlsListener := &ShadowTLSListener{
		inner:  tcpListener,
		server: t.server,
		log:    t.log,
		connCh: make(chan net.Conn, 256),
		done:   make(chan struct{}),
	}

	go stlsListener.acceptLoop()

	t.listener = stlsListener
	t.log.Info("ShadowTLS transport listening on %s (handshake: %s)", addr, t.cfg.Reality.Dest)
	return stlsListener, nil
}

// Close shuts down the transport
func (t *ShadowTLSTransport) Close() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

// ShadowTLSListener wraps a listener with ShadowTLS protocol
type ShadowTLSListener struct {
	inner  net.Listener
	server *antidpi.ShadowTLSServer
	log    *logger.Logger
	connCh chan net.Conn
	done   chan struct{}
}

func (l *ShadowTLSListener) acceptLoop() {
	for {
		select {
		case <-l.done:
			return
		default:
		}

		conn, err := l.inner.Accept()
		if err != nil {
			select {
			case <-l.done:
				return
			default:
				l.log.Error("ShadowTLS accept error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		go l.handleConn(conn)
	}
}

func (l *ShadowTLSListener) handleConn(conn net.Conn) {
	tunnelConn, err := l.server.HandleConnection(conn)
	if err != nil {
		l.log.Debug("ShadowTLS: connection rejected: %v", err)
		conn.Close()
		return
	}

	select {
	case l.connCh <- tunnelConn:
	case <-l.done:
		tunnelConn.Close()
	}
}

func (l *ShadowTLSListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.done:
		return nil, fmt.Errorf("listener closed")
	}
}

func (l *ShadowTLSListener) Close() error {
	close(l.done)
	return l.inner.Close()
}

func (l *ShadowTLSListener) Addr() net.Addr {
	return l.inner.Addr()
}
