package server

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/crypto"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/mux"
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

	return &Server{
		cfg:       cfg,
		log:       log,
		transport: tp,
		encryptor: enc,
		done:      make(chan struct{}),
	}, nil
}

// Start begins accepting connections
func (s *Server) Start() error {
	listener, err := s.transport.Listen()
	if err != nil {
		return err
	}

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
	s.log.Info("New connection from %s", remoteAddr)

	// Perform handshake (authenticate client)
	if err := s.handshake(conn); err != nil {
		s.log.Warn("Handshake failed from %s: %v", remoteAddr, err)
		return
	}

	// Create mux session
	session, err := mux.NewServerSession(conn, &s.cfg.Mux, s.log)
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

		go s.handleStream(stream)
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

	// Relay data bidirectionally
	mux.Relay(stream, destConn)
}

// Stop gracefully shuts down the server
func (s *Server) Stop() {
	close(s.done)
	s.transport.Close()
}
