package transport

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/utils"
)

// TCPMux implements Transport using raw TCP with optional TLS
type TCPMux struct {
	cfg      *config.Config
	log      *logger.Logger
	listener net.Listener
}

// NewTCPMux creates a new TCP Mux transport
func NewTCPMux(cfg *config.Config, log *logger.Logger) *TCPMux {
	return &TCPMux{
		cfg: cfg,
		log: log,
	}
}

// Name returns the transport name
func (t *TCPMux) Name() string {
	return "tcpmux"
}

// Dial connects to the remote server
func (t *TCPMux) Dial() (net.Conn, error) {
	addr := t.cfg.RemoteAddr

	// Set dial timeout
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: time.Duration(t.cfg.Performance.KeepAlive) * time.Second,
	}

	var conn net.Conn
	var err error

	if t.cfg.TLSCert != "" || t.cfg.TLSKey != "" {
		// TLS connection
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, // TODO: proper cert validation
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		// Plain TCP
		conn, err = dialer.Dial("tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("dial %s failed: %w", addr, err)
	}

	utils.OptimizeTCP(conn, t.cfg.Performance)

	t.log.Debug("Connected to %s via TCP", addr)
	return conn, nil
}

// Listen starts accepting TCP connections
func (t *TCPMux) Listen() (net.Listener, error) {
	addr := t.cfg.BindAddr

	var listener net.Listener

	if t.cfg.TLSCert != "" && t.cfg.TLSKey != "" {
		// TLS listener
		cert, err := tls.LoadX509KeyPair(t.cfg.TLSCert, t.cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS cert: %w", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		var listenErr error
		listener, listenErr = tls.Listen("tcp", addr, tlsConfig)
		if listenErr != nil {
			return nil, fmt.Errorf("TLS listen on %s failed: %w", addr, listenErr)
		}
	} else {
		// Plain TCP listener
		var err error
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("listen on %s failed: %w", addr, err)
		}
	}

	t.listener = listener
	t.log.Info("TCP transport listening on %s", addr)
	return utils.NewOptimizedListener(listener, t.cfg.Performance), nil
}

// Close shuts down the transport
func (t *TCPMux) Close() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

