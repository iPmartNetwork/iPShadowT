package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// QUICMuxSession wraps a QUIC connection with native multi-stream support
// Unlike the basic QUIC transport (single stream), this uses QUIC's built-in
// multiplexing — each logical stream maps to a QUIC stream (no smux needed)
//
// Advantages over smux-over-QUIC:
// - No head-of-line blocking between streams
// - Native flow control per stream
// - Lower overhead (no extra framing)
// - Connection migration support
type QUICMuxSession struct {
	conn          *quic.Conn
	log           *logger.Logger
	streams       sync.Map
	streamCount   atomic.Int64
	maxStreams    int
	closed        atomic.Bool
}

// NewQUICMuxSession creates a mux session from a QUIC connection
func NewQUICMuxSession(conn *quic.Conn, maxStreams int, log *logger.Logger) *QUICMuxSession {
	if maxStreams <= 0 {
		maxStreams = 1024
	}
	return &QUICMuxSession{
		conn:       conn,
		log:        log,
		maxStreams: maxStreams,
	}
}

// OpenStream opens a new QUIC stream (client-side)
func (s *QUICMuxSession) OpenStream() (net.Conn, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("session closed")
	}

	if int(s.streamCount.Load()) >= s.maxStreams {
		return nil, fmt.Errorf("max streams reached (%d)", s.maxStreams)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := s.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}

	s.streamCount.Add(1)
	sc := &quicMuxStreamConn{
		stream:  stream,
		session: s,
		local:   s.conn.LocalAddr(),
		remote:  s.conn.RemoteAddr(),
	}
	return sc, nil
}

// AcceptStream accepts an incoming QUIC stream (server-side)
func (s *QUICMuxSession) AcceptStream() (net.Conn, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("session closed")
	}

	ctx := context.Background()
	stream, err := s.conn.AcceptStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("accept stream: %w", err)
	}

	s.streamCount.Add(1)
	sc := &quicMuxStreamConn{
		stream:  stream,
		session: s,
		local:   s.conn.LocalAddr(),
		remote:  s.conn.RemoteAddr(),
	}
	return sc, nil
}

// Close closes the QUIC mux session
func (s *QUICMuxSession) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	return s.conn.CloseWithError(0, "session closed")
}

// IsClosed returns whether the session is closed
func (s *QUICMuxSession) IsClosed() bool {
	return s.closed.Load()
}

// StreamCount returns the number of active streams
func (s *QUICMuxSession) StreamCount() int {
	return int(s.streamCount.Load())
}

// LocalAddr returns the local address
func (s *QUICMuxSession) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

// RemoteAddr returns the remote address
func (s *QUICMuxSession) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

// quicMuxStreamConn wraps a single QUIC stream as net.Conn
type quicMuxStreamConn struct {
	stream  *quic.Stream
	session *QUICMuxSession
	local   net.Addr
	remote  net.Addr
	closed  atomic.Bool
}

func (c *quicMuxStreamConn) Read(p []byte) (int, error) {
	return c.stream.Read(p)
}

func (c *quicMuxStreamConn) Write(p []byte) (int, error) {
	return c.stream.Write(p)
}

func (c *quicMuxStreamConn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	c.session.streamCount.Add(-1)
	return c.stream.Close()
}

func (c *quicMuxStreamConn) LocalAddr() net.Addr                { return c.local }
func (c *quicMuxStreamConn) RemoteAddr() net.Addr               { return c.remote }
func (c *quicMuxStreamConn) SetDeadline(t time.Time) error      { c.stream.SetDeadline(t); return nil }
func (c *quicMuxStreamConn) SetReadDeadline(t time.Time) error  { c.stream.SetReadDeadline(t); return nil }
func (c *quicMuxStreamConn) SetWriteDeadline(t time.Time) error { c.stream.SetWriteDeadline(t); return nil }

// QUICMuxPool manages multiple QUIC mux sessions
type QUICMuxPool struct {
	sessions []*QUICMuxSession
	mu       sync.RWMutex
	current  atomic.Int64
	cfg      *config.MuxConfig
	log      *logger.Logger
}

// NewQUICMuxPool creates a pool of QUIC mux sessions
func NewQUICMuxPool(cfg *config.MuxConfig, log *logger.Logger) *QUICMuxPool {
	return &QUICMuxPool{
		sessions: make([]*QUICMuxSession, 0),
		cfg:      cfg,
		log:      log,
	}
}

// Add adds a session to the pool
func (p *QUICMuxPool) Add(session *QUICMuxSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessions = append(p.sessions, session)
}

// GetStream gets a stream from the least-loaded session
func (p *QUICMuxPool) GetStream() (net.Conn, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.sessions) == 0 {
		return nil, fmt.Errorf("no sessions available")
	}

	// Find session with fewest streams
	var best *QUICMuxSession
	var bestCount int = int(^uint(0) >> 1)

	for _, s := range p.sessions {
		if s.IsClosed() {
			continue
		}
		count := s.StreamCount()
		if count < bestCount {
			bestCount = count
			best = s
		}
	}

	if best == nil {
		return nil, fmt.Errorf("all sessions closed")
	}

	return best.OpenStream()
}

// Close closes all sessions in the pool
func (p *QUICMuxPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.sessions {
		s.Close()
	}
	p.sessions = nil
}

// Relay copies data bidirectionally between two ReadWriteClosers
func Relay(a, b io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(b, a) }()
	go func() { defer wg.Done(); io.Copy(a, b) }()
	wg.Wait()
}
