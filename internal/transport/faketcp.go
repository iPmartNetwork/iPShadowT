package transport

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// FakeTCPTransport implements UDP-over-fake-TCP tunneling
//
// Inspired by: dndx/phantun (Rust) — adapted concept to Go
//
// How it works:
// 1. UDP packets (e.g., WireGuard, QUIC, KCP) are encapsulated
// 2. Sent over a connection that LOOKS like TCP to firewalls
// 3. Layer 3/4 firewalls see "TCP" traffic and allow it
// 4. But it's actually UDP payload inside fake TCP framing
//
// Use case:
// - When UDP is completely blocked but TCP is allowed
// - WireGuard/QUIC/KCP traffic needs to pass through
// - Combines with other transports as an outer wrapper
//
// Framing protocol:
// [2 bytes: payload length][N bytes: UDP payload]
type FakeTCPTransport struct {
	cfg      *config.Config
	log      *logger.Logger
	listener net.Listener
	mu       sync.Mutex
}

// NewFakeTCP creates a new fake TCP transport
func NewFakeTCP(cfg *config.Config, log *logger.Logger) *FakeTCPTransport {
	return &FakeTCPTransport{
		cfg: cfg,
		log: log,
	}
}

// Name returns the transport name
func (f *FakeTCPTransport) Name() string {
	return "faketcp"
}

// Dial connects to the server — UDP payload over TCP connection
func (f *FakeTCPTransport) Dial() (net.Conn, error) {
	// Connect via TCP (firewalls see normal TCP SYN/ACK)
	conn, err := net.DialTimeout("tcp", f.cfg.RemoteAddr, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("fakeTCP dial failed: %w", err)
	}

	f.log.Debug("FakeTCP connected to %s", f.cfg.RemoteAddr)
	return &fakeTCPConn{
		Conn: conn,
		log:  f.log,
	}, nil
}

// Listen starts accepting fake TCP connections
func (f *FakeTCPTransport) Listen() (net.Listener, error) {
	ln, err := net.Listen("tcp", f.cfg.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("fakeTCP listen failed: %w", err)
	}

	f.mu.Lock()
	f.listener = ln
	f.mu.Unlock()

	f.log.Info("FakeTCP listening on %s (TCP carrying UDP payloads)", f.cfg.BindAddr)
	return &fakeTCPListener{
		listener: ln,
		log:      f.log,
	}, nil
}

// Close shuts down the transport
func (f *FakeTCPTransport) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener != nil {
		return f.listener.Close()
	}
	return nil
}

// fakeTCPListener wraps net.Listener to return fakeTCPConn
type fakeTCPListener struct {
	listener net.Listener
	log      *logger.Logger
}

func (l *fakeTCPListener) Accept() (net.Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	return &fakeTCPConn{Conn: conn, log: l.log}, nil
}

func (l *fakeTCPListener) Close() error { return l.listener.Close() }
func (l *fakeTCPListener) Addr() net.Addr { return l.listener.Addr() }

// fakeTCPConn wraps a TCP connection with length-prefixed framing
// Each "UDP packet" is sent as [2-byte length][payload]
// This preserves message boundaries over the TCP stream
type fakeTCPConn struct {
	net.Conn
	log       *logger.Logger
	readBuf   []byte
	readPos   int
	readLen   int
	writeMu   sync.Mutex
	readMu    sync.Mutex
	bytesSent atomic.Int64
	bytesRecv atomic.Int64
}

// Write sends a UDP-like message over the TCP connection with length prefix
func (c *fakeTCPConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if len(p) > 65535 {
		return 0, fmt.Errorf("payload too large: %d > 65535", len(p))
	}

	// Write length header (2 bytes, big-endian)
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(p)))

	// Write header + payload atomically
	buf := make([]byte, 2+len(p))
	copy(buf[:2], header)
	copy(buf[2:], p)

	n, err := c.Conn.Write(buf)
	if err != nil {
		return 0, err
	}
	if n < len(buf) {
		return 0, fmt.Errorf("short write: %d < %d", n, len(buf))
	}

	c.bytesSent.Add(int64(len(p)))
	return len(p), nil
}

// Read reads a complete UDP-like message from the TCP connection
func (c *fakeTCPConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	// If we have buffered data from a previous read, return it
	if c.readPos < c.readLen {
		n := copy(p, c.readBuf[c.readPos:c.readLen])
		c.readPos += n
		return n, nil
	}

	// Read length header
	header := make([]byte, 2)
	if _, err := readFull(c.Conn, header); err != nil {
		return 0, err
	}

	payloadLen := int(binary.BigEndian.Uint16(header))
	if payloadLen == 0 {
		return 0, nil
	}
	if payloadLen > 65535 {
		return 0, fmt.Errorf("invalid payload length: %d", payloadLen)
	}

	// Read payload
	if cap(c.readBuf) < payloadLen {
		c.readBuf = make([]byte, payloadLen)
	}
	c.readBuf = c.readBuf[:payloadLen]

	if _, err := readFull(c.Conn, c.readBuf); err != nil {
		return 0, err
	}

	c.readLen = payloadLen
	c.readPos = 0

	// Copy to caller's buffer
	n := copy(p, c.readBuf)
	c.readPos = n
	c.bytesRecv.Add(int64(n))
	return n, nil
}

// Stats returns bytes sent/received
func (c *fakeTCPConn) Stats() (sent, recv int64) {
	return c.bytesSent.Load(), c.bytesRecv.Load()
}

// readFull reads exactly len(buf) bytes from reader
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
