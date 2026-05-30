package transport

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// ReverseTransport implements a reverse tunnel where the server (Iran/behind NAT)
// initiates the connection to the relay (abroad), not the other way around.
//
// Problem: Iran server is behind NAT → can't accept incoming connections
// Solution: Iran server connects OUT to the foreign server and registers itself
//
// Architecture:
//   Iran Server (behind NAT) ──connects to──→ Foreign Relay Server
//   Client ──connects to──→ Foreign Relay Server ──forwards to──→ Iran Server
//
// This is similar to Xray's VLESS Reverse or ngrok's model.
type ReverseTransport struct {
	cfg      *config.Config
	log      *logger.Logger
	inner    Transport // The actual transport used for the reverse connection
	connPool chan net.Conn
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewReverse creates a new reverse transport
func NewReverse(cfg *config.Config, log *logger.Logger) (*ReverseTransport, error) {
	// Create the inner transport (what we use to connect to the relay)
	innerCfg := *cfg
	// The inner transport connects to the relay server
	inner, err := NewTransport(&innerCfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create inner transport: %w", err)
	}

	return &ReverseTransport{
		cfg:      cfg,
		log:      log,
		inner:    inner,
		connPool: make(chan net.Conn, 32),
		done:     make(chan struct{}),
	}, nil
}

// Name returns the transport name
func (r *ReverseTransport) Name() string {
	return "reverse"
}

// Dial is not used in reverse mode (server initiates connection)
func (r *ReverseTransport) Dial() (net.Conn, error) {
	return r.inner.Dial()
}

// Listen returns a listener that provides connections from the reverse pool
// The "server" (Iran) calls this, but instead of accepting incoming connections,
// it dials OUT to the relay and registers itself
func (r *ReverseTransport) Listen() (net.Listener, error) {
	// Start the reverse connection maintainer
	r.wg.Add(1)
	go r.maintainReverseConnections()

	listener := &ReverseListener{
		connCh: r.connPool,
		done:   r.done,
		addr:   r.cfg.BindAddr,
	}

	r.log.Info("Reverse transport: connecting to relay at %s", r.cfg.RemoteAddr)
	return listener, nil
}

// maintainReverseConnections keeps a pool of connections to the relay
func (r *ReverseTransport) maintainReverseConnections() {
	defer r.wg.Done()

	poolSize := 4 // Keep 4 connections ready
	if r.cfg.Pool.Size > 0 {
		poolSize = r.cfg.Pool.Size
	}

	for {
		select {
		case <-r.done:
			return
		default:
		}

		// Fill the pool
		currentSize := len(r.connPool)
		if currentSize < poolSize {
			conn, err := r.dialRelay()
			if err != nil {
				r.log.Debug("Reverse: dial relay failed: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			select {
			case r.connPool <- conn:
				r.log.Debug("Reverse: connection added to pool (%d/%d)", len(r.connPool), poolSize)
			case <-r.done:
				conn.Close()
				return
			default:
				// Pool is full
				conn.Close()
			}
		}

		time.Sleep(1 * time.Second)
	}
}

// dialRelay connects to the relay server and registers as a backend
func (r *ReverseTransport) dialRelay() (net.Conn, error) {
	conn, err := r.inner.Dial()
	if err != nil {
		return nil, err
	}

	// Send registration message
	// This tells the relay "I'm a backend server, send me client connections"
	regMsg := []byte("iPShadowT-REVERSE-REGISTER")
	header := make([]byte, 2+len(regMsg))
	header[0] = byte(len(regMsg) >> 8)
	header[1] = byte(len(regMsg))
	copy(header[2:], regMsg)

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(header); err != nil {
		conn.Close()
		return nil, fmt.Errorf("registration failed: %w", err)
	}
	conn.SetWriteDeadline(time.Time{})

	// Wait for acknowledgment
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	ack := make([]byte, 2)
	if _, err := conn.Read(ack); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ack failed: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	if ack[0] != 'O' || ack[1] != 'K' {
		conn.Close()
		return nil, fmt.Errorf("relay rejected registration")
	}

	return conn, nil
}

// Close shuts down the reverse transport
func (r *ReverseTransport) Close() error {
	close(r.done)
	r.wg.Wait()

	// Drain and close pool
	for {
		select {
		case conn := <-r.connPool:
			conn.Close()
		default:
			return r.inner.Close()
		}
	}
}

// ReverseListener implements net.Listener using the reverse connection pool
type ReverseListener struct {
	connCh chan net.Conn
	done   chan struct{}
	addr   string
}

func (l *ReverseListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.done:
		return nil, fmt.Errorf("listener closed")
	}
}

func (l *ReverseListener) Close() error {
	return nil // Closed by parent
}

func (l *ReverseListener) Addr() net.Addr {
	addr, _ := net.ResolveTCPAddr("tcp", l.addr)
	return addr
}
