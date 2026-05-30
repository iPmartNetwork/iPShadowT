package mux

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/xtaci/smux"
)

// Session wraps a smux session with connection management
type Session struct {
	session *smux.Session
	conn    net.Conn
	config  *smux.Config
	log     *logger.Logger
	mu      sync.Mutex
	closed  bool
}

// NewClientSession creates a new mux session on a connection (client-side)
func NewClientSession(conn net.Conn, cfg *config.MuxConfig, log *logger.Logger) (*Session, error) {
	smuxCfg := buildSmuxConfig(cfg)

	session, err := smux.Client(conn, smuxCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create mux client session: %w", err)
	}

	return &Session{
		session: session,
		conn:    conn,
		config:  smuxCfg,
		log:     log,
	}, nil
}

// NewServerSession creates a new mux session on a connection (server-side)
func NewServerSession(conn net.Conn, cfg *config.MuxConfig, log *logger.Logger) (*Session, error) {
	smuxCfg := buildSmuxConfig(cfg)

	session, err := smux.Server(conn, smuxCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create mux server session: %w", err)
	}

	return &Session{
		session: session,
		conn:    conn,
		config:  smuxCfg,
		log:     log,
	}, nil
}

// OpenStream opens a new stream on the mux session
func (s *Session) OpenStream() (*smux.Stream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("session is closed")
	}

	stream, err := s.session.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	return stream, nil
}

// AcceptStream accepts a new stream from the mux session
func (s *Session) AcceptStream() (*smux.Stream, error) {
	return s.session.AcceptStream()
}

// IsClosed returns whether the session is closed
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed || s.session.IsClosed()
}

// NumStreams returns the number of active streams
func (s *Session) NumStreams() int {
	return s.session.NumStreams()
}

// Close closes the mux session and underlying connection
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	if err := s.session.Close(); err != nil {
		return err
	}
	return s.conn.Close()
}

// buildSmuxConfig creates smux config from our config
func buildSmuxConfig(cfg *config.MuxConfig) *smux.Config {
	smuxCfg := smux.DefaultConfig()

	if cfg.FrameSize > 0 {
		smuxCfg.MaxFrameSize = cfg.FrameSize
	}
	if cfg.RecvBuffer > 0 {
		smuxCfg.MaxReceiveBuffer = cfg.RecvBuffer
	}
	if cfg.StreamBuffer > 0 {
		smuxCfg.MaxStreamBuffer = cfg.StreamBuffer
	}

	// Keep alive
	smuxCfg.KeepAliveInterval = 10 * time.Second
	smuxCfg.KeepAliveTimeout = 30 * time.Second

	return smuxCfg
}

// SessionPool manages multiple mux sessions for load distribution
type SessionPool struct {
	sessions []*Session
	mu       sync.RWMutex
	index    int
	cfg      *config.MuxConfig
	log      *logger.Logger
}

// NewSessionPool creates a new session pool
func NewSessionPool(cfg *config.MuxConfig, log *logger.Logger) *SessionPool {
	return &SessionPool{
		sessions: make([]*Session, 0),
		cfg:      cfg,
		log:      log,
	}
}

// Add adds a session to the pool
func (p *SessionPool) Add(session *Session) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessions = append(p.sessions, session)
}

// GetStream gets a stream from the least-loaded session
func (p *SessionPool) GetStream() (*smux.Stream, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.sessions) == 0 {
		return nil, fmt.Errorf("no sessions available")
	}

	// Find session with least streams (load balancing)
	var bestSession *Session
	minStreams := int(^uint(0) >> 1) // max int

	for _, s := range p.sessions {
		if s.IsClosed() {
			continue
		}
		if n := s.NumStreams(); n < minStreams {
			minStreams = n
			bestSession = s
		}
	}

	if bestSession == nil {
		return nil, fmt.Errorf("all sessions are closed")
	}

	// Check if we should limit streams per session
	if p.cfg.MaxStreams > 0 && minStreams >= p.cfg.MaxStreams {
		return nil, fmt.Errorf("all sessions at max capacity (%d streams)", p.cfg.MaxStreams)
	}

	return bestSession.OpenStream()
}

// RemoveClosed removes closed sessions from the pool
func (p *SessionPool) RemoveClosed() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	active := make([]*Session, 0, len(p.sessions))
	removed := 0

	for _, s := range p.sessions {
		if s.IsClosed() {
			removed++
		} else {
			active = append(active, s)
		}
	}

	p.sessions = active
	return removed
}

// Count returns the number of active sessions
func (p *SessionPool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, s := range p.sessions {
		if !s.IsClosed() {
			count++
		}
	}
	return count
}

// Close closes all sessions in the pool
func (p *SessionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, s := range p.sessions {
		s.Close()
	}
	p.sessions = nil
}

// Relay copies data between two streams bidirectionally
func Relay(left, right io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(right, left)
		if closer, ok := right.(interface{ CloseWrite() error }); ok {
			closer.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		io.Copy(left, right)
		if closer, ok := left.(interface{ CloseWrite() error }); ok {
			closer.CloseWrite()
		}
	}()

	wg.Wait()
}
