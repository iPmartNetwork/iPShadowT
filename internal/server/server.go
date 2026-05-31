package server

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/crypto"
	"github.com/iPmart/iPShadowT/internal/health"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/metrics"
	"github.com/iPmart/iPShadowT/internal/mux"
	"github.com/iPmart/iPShadowT/internal/plugin"
	"github.com/iPmart/iPShadowT/internal/ratelimit"
	"github.com/iPmart/iPShadowT/internal/security"
	"github.com/iPmart/iPShadowT/internal/transport"
)

// Server handles incoming tunnel connections
type Server struct {
	cfg       *config.Config
	log       *logger.Logger
	transport transport.Transport
	encryptor *crypto.Encryptor
	sessions  sync.Map // map of active sessions
	done      chan struct{}

	// Integrated modules
	healthSvc  *health.Service
	metrics    *metrics.Collector
	limiter    *ratelimit.Limiter
	security   *security.Manager
	plugins    *plugin.Registry
}

// New creates a new server instance
func New(cfg *config.Config, log *logger.Logger) (*Server, error) {
	// Create transport
	tp, err := transport.NewTransport(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	// Create encryptor
	enc, err := crypto.NewEncryptor(cfg.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	s := &Server{
		cfg:       cfg,
		log:       log,
		transport: tp,
		encryptor: enc,
		done:      make(chan struct{}),
	}

	// Initialize metrics collector
	s.metrics = metrics.NewCollector(log)

	// Initialize rate limiter
	s.limiter = ratelimit.NewLimiter(log)

	// Initialize security manager
	s.security = security.NewManager(log)

	// Initialize plugin registry
	s.plugins = plugin.NewRegistry(log)

	// Initialize health service if configured
	if cfg.Health.Enabled && cfg.Health.Listen != "" {
		s.healthSvc = health.NewService(cfg.Health.Listen, log)
	}

	return s, nil
}

// Start begins accepting connections
func (s *Server) Start() error {
	listener, err := s.transport.Listen()
	if err != nil {
		return err
	}

	// Start health service
	if s.healthSvc != nil {
		if err := s.healthSvc.Start(); err != nil {
			s.log.Warn("Health service failed to start: %v", err)
		}
	}

	// Start metrics endpoint
	if s.metrics != nil {
		// Metrics on health port + 1 or default 9091
		metricsAddr := "127.0.0.1:9091"
		s.metrics.ServePrometheus(metricsAddr)
	}

	// Start plugins
	s.plugins.StartAll()

	s.log.Info("Server listening on %s (transport: %s)", s.cfg.BindAddr, s.transport.Name())

	go s.acceptLoop(listener)

	return nil
}

// acceptLoop accepts new connections
func (s *Server) acceptLoop(listener net.Listener) {
	for {
		select {
		case <-s.done:
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				s.log.Error("Accept error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a new client connection
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()

	// Security check: IP whitelist/blacklist
	if !s.security.IsAllowed(remoteAddr) {
		s.log.Warn("Connection rejected (blacklisted): %s", remoteAddr)
		s.security.Audit("connection_rejected", remoteAddr, "", "IP blacklisted", false)
		return
	}

	// Metrics: track connection
	s.metrics.TotalConnections.Add(1)
	s.metrics.ActiveConnections.Add(1)
	defer s.metrics.ActiveConnections.Add(-1)

	// Health: track connection
	if s.healthSvc != nil {
		s.healthSvc.AddConnection()
		defer s.healthSvc.RemoveConnection()
	}

	s.log.Info("New connection from %s", remoteAddr)

	// Perform handshake (authenticate client)
	if err := s.handshake(conn); err != nil {
		s.log.Warn("Handshake failed from %s: %v", remoteAddr, err)
		s.metrics.FailedConnections.Add(1)

		// Brute force protection
		blocked := s.security.RecordFailedLogin(remoteAddr)
		if blocked {
			s.log.Warn("IP blocked due to brute force: %s", remoteAddr)
		}
		return
	}

	// Reset failed login counter on success
	s.security.ResetFailedLogins(remoteAddr)
	s.security.Audit("connection_accepted", remoteAddr, "", "Handshake OK", true)

	// Apply rate limiting (wrap connection)
	wrappedConn := s.limiter.WrapConn(conn, remoteAddr)

	// Run plugin filters
	filteredConn, err := s.plugins.RunFilters(wrappedConn)
	if err != nil {
		s.log.Warn("Plugin filter rejected connection from %s: %v", remoteAddr, err)
		return
	}

	// Create mux session
	session, err := mux.NewServerSession(filteredConn, &s.cfg.Mux, s.log)
	if err != nil {
		s.log.Error("Failed to create mux session from %s: %v", remoteAddr, err)
		return
	}
	defer session.Close()

	s.log.Info("Mux session established with %s", remoteAddr)

	// Accept streams
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if !session.IsClosed() {
				s.log.Debug("Stream accept error from %s: %v", remoteAddr, err)
			}
			return
		}

		// Track streams
		s.metrics.TotalStreams.Add(1)
		s.metrics.ActiveStreams.Add(1)
		if s.healthSvc != nil {
			s.healthSvc.AddStream()
		}

		go func() {
			defer s.metrics.ActiveStreams.Add(-1)
			defer func() {
				if s.healthSvc != nil {
					s.healthSvc.RemoveStream()
				}
			}()
			s.handleStream(stream)
		}()
	}
}

// handshake performs the authentication handshake
func (s *Server) handshake(conn net.Conn) error {
	// Set deadline for handshake
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetDeadline(time.Time{})

	// Read client hello (encrypted)
	hello, err := crypto.ReadEncryptedFrame(conn, s.encryptor)
	if err != nil {
		return fmt.Errorf("failed to read hello: %w", err)
	}

	// Verify hello message
	if string(hello) != "iPShadowT-HELLO" {
		return fmt.Errorf("invalid hello message")
	}

	// Send server response
	if err := crypto.WriteEncryptedFrame(conn, s.encryptor, []byte("iPShadowT-OK")); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}

	return nil
}

// handleStream handles a single mux stream (port forwarding)
func (s *Server) handleStream(stream io.ReadWriteCloser) {
	defer stream.Close()

	// Read the destination address from the stream header
	destBuf := make([]byte, 2)
	if _, err := io.ReadFull(stream, destBuf); err != nil {
		s.log.Debug("Failed to read dest length: %v", err)
		return
	}

	destLen := int(destBuf[0])<<8 | int(destBuf[1])
	if destLen > 512 {
		s.log.Warn("Invalid destination length: %d", destLen)
		return
	}

	destAddr := make([]byte, destLen)
	if _, err := io.ReadFull(stream, destAddr); err != nil {
		s.log.Debug("Failed to read dest addr: %v", err)
		return
	}

	dest := string(destAddr)
	s.log.Debug("Forwarding stream to %s", dest)

	// Connect to destination
	destConn, err := net.DialTimeout("tcp", dest, 10*time.Second)
	if err != nil {
		s.log.Debug("Failed to connect to %s: %v", dest, err)
		return
	}
	defer destConn.Close()

	// Relay data bidirectionally (with traffic accounting)
	s.relayWithMetrics(stream, destConn)
}

// relayWithMetrics relays data between two connections while tracking bytes
func (s *Server) relayWithMetrics(a io.ReadWriteCloser, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// a → b (download from perspective of client)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(b, a)
		s.metrics.BytesReceived.Add(n)
		if s.healthSvc != nil {
			s.healthSvc.AddBytesIn(n)
		}
	}()

	// b → a (upload from perspective of client)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(a, b)
		s.metrics.BytesSent.Add(n)
		if s.healthSvc != nil {
			s.healthSvc.AddBytesOut(n)
		}
	}()

	wg.Wait()
}

// GetMetrics returns the metrics collector
func (s *Server) GetMetrics() *metrics.Collector {
	return s.metrics
}

// GetHealth returns the health service
func (s *Server) GetHealth() *health.Service {
	return s.healthSvc
}

// GetSecurity returns the security manager
func (s *Server) GetSecurity() *security.Manager {
	return s.security
}

// GetPlugins returns the plugin registry
func (s *Server) GetPlugins() *plugin.Registry {
	return s.plugins
}

// SetRateLimit sets bandwidth limit for a user/IP
func (s *Server) SetRateLimit(userID string, bytesPerSec int64) {
	s.limiter.SetLimit(userID, bytesPerSec, bytesPerSec*2)
	s.log.Info("Rate limit set for %s: %d bytes/sec", userID, bytesPerSec)
}

// Stop gracefully shuts down the server
func (s *Server) Stop() {
	close(s.done)
	s.transport.Close()

	// Stop health service
	if s.healthSvc != nil {
		s.healthSvc.Stop()
	}

	// Stop plugins
	s.plugins.StopAll()

	// Cleanup expired blocks
	s.security.CleanupExpiredBlocks()
}
