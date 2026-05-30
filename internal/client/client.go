package client

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/crypto"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/mux"
	"github.com/iPmart/iPShadowT/internal/transport"
	"github.com/iPmart/iPShadowT/internal/tunnel"
)

// Client manages the tunnel connection to the server
type Client struct {
	cfg       *config.Config
	log       *logger.Logger
	transport transport.Transport
	encryptor *crypto.Encryptor
	pool      *mux.SessionPool
	forwards  []*tunnel.Forwarder
	done      chan struct{}
	wg        sync.WaitGroup
}

// New creates a new client instance
func New(cfg *config.Config, log *logger.Logger) (*Client, error) {
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

	return &Client{
		cfg:       cfg,
		log:       log,
		transport: tp,
		encryptor: enc,
		pool:      mux.NewSessionPool(&cfg.Mux, log),
		done:      make(chan struct{}),
	}, nil
}

// Start connects to the server and starts port forwarding
func (c *Client) Start() error {
	// Establish initial mux sessions
	if err := c.connectSessions(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Start port forwarders
	if err := c.startForwarders(); err != nil {
		return fmt.Errorf("failed to start forwarders: %w", err)
	}

	// Start session maintenance (reconnect, health check)
	c.wg.Add(1)
	go c.maintainSessions()

	return nil
}

// connectSessions establishes mux sessions to the server
func (c *Client) connectSessions() error {
	concurrency := c.cfg.Mux.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	c.log.Info("Establishing %d mux sessions...", concurrency)

	var firstErr error
	connected := 0

	for i := 0; i < concurrency; i++ {
		session, err := c.createSession()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			c.log.Warn("Session %d failed: %v", i+1, err)
			continue
		}
		c.pool.Add(session)
		connected++
	}

	if connected == 0 {
		return fmt.Errorf("failed to establish any session: %w", firstErr)
	}

	c.log.Info("✅ Connected with %d/%d mux sessions", connected, concurrency)
	return nil
}

// createSession creates a single mux session
func (c *Client) createSession() (*mux.Session, error) {
	// Dial the server
	conn, err := c.transport.Dial()
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	// Perform handshake
	if err := c.handshake(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	// Create mux session
	session, err := mux.NewClientSession(conn, &c.cfg.Mux, c.log)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("mux session failed: %w", err)
	}

	return session, nil
}

// handshake performs authentication with the server
func (c *Client) handshake(conn net.Conn) error {
	// Set deadline for handshake
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetDeadline(time.Time{})

	// Send hello
	if err := crypto.WriteEncryptedFrame(conn, c.encryptor, []byte("iPShadowT-HELLO")); err != nil {
		return fmt.Errorf("failed to send hello: %w", err)
	}

	// Read server response
	resp, err := crypto.ReadEncryptedFrame(conn, c.encryptor)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if string(resp) != "iPShadowT-OK" {
		return fmt.Errorf("server rejected connection: %s", string(resp))
	}

	return nil
}

// startForwarders starts all configured port forwarders
func (c *Client) startForwarders() error {
	if len(c.cfg.Forwards) == 0 {
		c.log.Warn("No port forwards configured")
		return nil
	}

	for _, fwdCfg := range c.cfg.Forwards {
		fwd, err := tunnel.NewForwarder(fwdCfg, c.pool, c.log)
		if err != nil {
			return fmt.Errorf("failed to create forwarder %q: %w", fwdCfg.Name, err)
		}

		if err := fwd.Start(); err != nil {
			return fmt.Errorf("failed to start forwarder %q: %w", fwdCfg.Name, err)
		}

		c.forwards = append(c.forwards, fwd)
		c.log.Info("  📡 Forward: %s [%s] %s → %s", fwdCfg.Name, fwdCfg.Type, fwdCfg.Listen, fwdCfg.Remote)
	}

	return nil
}

// maintainSessions keeps sessions alive and reconnects if needed
func (c *Client) maintainSessions() {
	defer c.wg.Done()

	ticker := time.NewTicker(time.Duration(c.cfg.Heartbeat.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.checkAndReconnect()
		}
	}
}

// checkAndReconnect checks session health and reconnects if needed
func (c *Client) checkAndReconnect() {
	// Remove closed sessions
	removed := c.pool.RemoveClosed()
	if removed > 0 {
		c.log.Warn("Removed %d dead sessions", removed)
	}

	// Check if we need more sessions
	active := c.pool.Count()
	target := c.cfg.Mux.Concurrency
	if target <= 0 {
		target = 4
	}

	if active < target {
		needed := target - active
		c.log.Info("Reconnecting %d sessions (active: %d, target: %d)", needed, active, target)

		for i := 0; i < needed; i++ {
			session, err := c.createSession()
			if err != nil {
				c.log.Error("Reconnect failed: %v", err)
				continue
			}
			c.pool.Add(session)
			c.log.Info("✅ Session reconnected (%d/%d)", active+i+1, target)
		}
	}
}

// Stop gracefully shuts down the client
func (c *Client) Stop() {
	close(c.done)

	// Stop forwarders
	for _, fwd := range c.forwards {
		fwd.Stop()
	}

	// Close session pool
	c.pool.Close()

	// Close transport
	c.transport.Close()

	c.wg.Wait()
	c.log.Info("Client stopped")
}
