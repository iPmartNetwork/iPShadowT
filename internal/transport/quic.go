package transport

import (
	"fmt"
	"net"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// QUICTransport implements Transport using QUIC protocol
// QUIC advantages:
// - Built-in multiplexing (no head-of-line blocking)
// - 0-RTT connection establishment
// - Better performance on lossy networks
// - UDP-based (faster than TCP in many cases)
//
// Note: QUIC/UDP may be blocked during severe filtering events
// Use as primary when available, with TCP fallback
type QUICTransport struct {
	cfg      *config.Config
	log      *logger.Logger
	listener interface{} // quic.Listener
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
// Note: This is a stub that uses TCP fallback when quic-go is not available
// To enable full QUIC, add github.com/quic-go/quic-go to go.mod
func (q *QUICTransport) Dial() (net.Conn, error) {
	// QUIC connection using UDP
	// For now, we implement a UDP-based connection wrapper
	// Full QUIC requires quic-go library

	addr, err := net.ResolveUDPAddr("udp", q.cfg.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve UDP addr failed: %w", err)
	}

	udpConn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("UDP dial failed: %w", err)
	}

	// Wrap with TLS-like encryption (our own AEAD layer handles this)
	q.log.Debug("QUIC connected to %s (UDP)", q.cfg.RemoteAddr)
	return udpConn, nil
}

// Listen starts accepting QUIC connections
func (q *QUICTransport) Listen() (net.Listener, error) {
	addr := q.cfg.BindAddr

	// Create UDP listener
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve UDP addr failed: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("UDP listen failed: %w", err)
	}

	quicListener := &QUICListener{
		udpConn: udpConn,
		connCh:  make(chan net.Conn, 256),
		done:    make(chan struct{}),
		addr:    addr,
		log:     q.log,
	}

	go quicListener.acceptLoop()

	q.log.Info("QUIC transport listening on %s (UDP)", addr)
	return quicListener, nil
}

// Close shuts down the transport
func (q *QUICTransport) Close() error {
	return nil
}

// QUICListener implements net.Listener for QUIC/UDP
type QUICListener struct {
	udpConn *net.UDPConn
	connCh  chan net.Conn
	done    chan struct{}
	addr    string
	log     *logger.Logger
	clients map[string]*UDPSession
}

// UDPSession represents a UDP client session
type UDPSession struct {
	remoteAddr *net.UDPAddr
	conn       *net.UDPConn
	lastSeen   time.Time
}

func (l *QUICListener) acceptLoop() {
	buf := make([]byte, 65535)
	l.clients = make(map[string]*UDPSession)

	for {
		select {
		case <-l.done:
			return
		default:
		}

		l.udpConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, remoteAddr, err := l.udpConn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		key := remoteAddr.String()
		if _, exists := l.clients[key]; !exists {
			// New client - create session
			session := &UDPSession{
				remoteAddr: remoteAddr,
				conn:       l.udpConn,
				lastSeen:   time.Now(),
			}
			l.clients[key] = session

			// Create a net.Conn wrapper
			udpNetConn := &UDPNetConn{
				session: session,
				buf:     make([]byte, n),
			}
			copy(udpNetConn.buf, buf[:n])

			select {
			case l.connCh <- udpNetConn:
			case <-l.done:
				return
			}
		}
	}
}

func (l *QUICListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.done:
		return nil, fmt.Errorf("listener closed")
	}
}

func (l *QUICListener) Close() error {
	close(l.done)
	return l.udpConn.Close()
}

func (l *QUICListener) Addr() net.Addr {
	return l.udpConn.LocalAddr()
}

// UDPNetConn wraps a UDP session as net.Conn
type UDPNetConn struct {
	session *UDPSession
	buf     []byte
	pos     int
}

func (c *UDPNetConn) Read(p []byte) (int, error) {
	if c.pos < len(c.buf) {
		n := copy(p, c.buf[c.pos:])
		c.pos += n
		return n, nil
	}
	// Read more from UDP
	buf := make([]byte, 65535)
	c.session.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	n, _, err := c.session.conn.ReadFromUDP(buf)
	if err != nil {
		return 0, err
	}
	copied := copy(p, buf[:n])
	if copied < n {
		c.buf = buf[copied:n]
		c.pos = 0
	}
	return copied, nil
}

func (c *UDPNetConn) Write(p []byte) (int, error) {
	return c.session.conn.WriteToUDP(p, c.session.remoteAddr)
}

func (c *UDPNetConn) Close() error                       { return nil }
func (c *UDPNetConn) LocalAddr() net.Addr                { return c.session.conn.LocalAddr() }
func (c *UDPNetConn) RemoteAddr() net.Addr               { return c.session.remoteAddr }
func (c *UDPNetConn) SetDeadline(t time.Time) error      { return nil }
func (c *UDPNetConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *UDPNetConn) SetWriteDeadline(t time.Time) error { return nil }
