package transport

import (
	"fmt"
	"net"
	"time"

	"github.com/iPmart/iPShadowT/internal/antidpi"
	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// Reality implements Transport using the REALITY protocol
// This is the most DPI-resistant transport:
// - Uses uTLS to mimic real browser TLS fingerprints
// - Server presents a real website to probes
// - Only authenticated clients can activate the tunnel
// - Traffic is indistinguishable from normal HTTPS
type Reality struct {
	cfg           *config.Config
	log           *logger.Logger
	listener      net.Listener
	realityServer *antidpi.RealityServer
	realityClient *antidpi.RealityClient
	antiDPI       *antidpi.Engine
}

// NewReality creates a new REALITY transport
func NewReality(cfg *config.Config, log *logger.Logger) (*Reality, error) {
	r := &Reality{
		cfg: cfg,
		log: log,
	}

	// Initialize anti-DPI engine
	engine, err := antidpi.NewEngine(&cfg.AntiDPI, &cfg.Reality, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create anti-DPI engine: %w", err)
	}
	r.antiDPI = engine

	// Initialize based on mode
	if cfg.Mode == "server" {
		realityCfg := antidpi.RealityConfig{
			Dest:       cfg.Reality.Dest,
			ServerName: cfg.Reality.ServerName,
			PrivateKey: cfg.Reality.PrivateKey,
			ShortIDs:   []string{cfg.Reality.ShortID},
		}
		server, err := antidpi.NewRealityServer(realityCfg, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create REALITY server: %w", err)
		}
		r.realityServer = server
	} else {
		realityCfg := antidpi.RealityConfig{
			ServerName:  cfg.Reality.ServerName,
			PublicKey:   cfg.Reality.PublicKey,
			ShortID:     cfg.Reality.ShortID,
			Fingerprint: cfg.AntiDPI.UTLSFingerprint,
		}
		client, err := antidpi.NewRealityClient(realityCfg, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create REALITY client: %w", err)
		}
		r.realityClient = client
	}

	return r, nil
}

// Name returns the transport name
func (r *Reality) Name() string {
	return "reality"
}

// Dial connects to the server using REALITY protocol
func (r *Reality) Dial() (net.Conn, error) {
	if r.realityClient == nil {
		return nil, fmt.Errorf("REALITY client not initialized")
	}

	// Step 1: TCP connection to server
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: time.Duration(r.cfg.Performance.KeepAlive) * time.Second,
	}

	tcpConn, err := dialer.Dial("tcp", r.cfg.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("TCP dial failed: %w", err)
	}

	// Step 2: Apply TLS fragmentation if enabled
	var conn net.Conn = tcpConn
	if r.cfg.AntiDPI.Fragment {
		fragmenter := r.antiDPI.GetFragmenter()
		if fragmenter != nil {
			conn = antidpi.NewFragmentedConn(conn, fragmenter)
		}
	}

	// Step 3: REALITY handshake (uTLS + auth)
	realityConn, err := r.realityClient.Connect(conn)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("REALITY connect failed: %w", err)
	}

	// Step 4: Apply traffic shaping if enabled
	if r.cfg.AntiDPI.TrafficShape {
		shaper := r.antiDPI.GetShaper()
		if shaper != nil {
			realityConn = shaper.WrapConn(realityConn)
		}
	}

	r.log.Debug("REALITY connection established to %s (SNI: %s)", r.cfg.RemoteAddr, r.cfg.Reality.ServerName)
	return realityConn, nil
}

// Listen starts accepting REALITY connections
func (r *Reality) Listen() (net.Listener, error) {
	addr := r.cfg.BindAddr

	// Listen on raw TCP (REALITY handles TLS itself)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s failed: %w", addr, err)
	}

	// Wrap with REALITY listener
	realityListener := &RealityListener{
		inner:         listener,
		realityServer: r.realityServer,
		log:           r.log,
		connCh:        make(chan net.Conn, 256),
		done:          make(chan struct{}),
	}

	// Start accepting and filtering connections
	go realityListener.acceptLoop()

	r.listener = realityListener
	r.log.Info("REALITY transport listening on %s (dest: %s)", addr, r.cfg.Reality.Dest)
	return realityListener, nil
}

// Close shuts down the transport
func (r *Reality) Close() error {
	if r.listener != nil {
		return r.listener.Close()
	}
	return nil
}

// RealityListener wraps a net.Listener with REALITY protocol handling
type RealityListener struct {
	inner         net.Listener
	realityServer *antidpi.RealityServer
	log           *logger.Logger
	connCh        chan net.Conn
	done          chan struct{}
}

// acceptLoop accepts connections and filters through REALITY
func (rl *RealityListener) acceptLoop() {
	for {
		select {
		case <-rl.done:
			return
		default:
		}

		conn, err := rl.inner.Accept()
		if err != nil {
			select {
			case <-rl.done:
				return
			default:
				rl.log.Error("REALITY accept error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		// Handle in goroutine (REALITY verification may take time)
		go rl.handleConn(conn)
	}
}

// handleConn processes a connection through REALITY
func (rl *RealityListener) handleConn(conn net.Conn) {
	// REALITY verification
	authedConn, isClient, err := rl.realityServer.HandleConnection(conn)
	if err != nil {
		rl.log.Debug("REALITY handle error: %v", err)
		conn.Close()
		return
	}

	if !isClient {
		// Connection was handled as fallback (probe)
		return
	}

	// Authenticated client - pass to tunnel
	select {
	case rl.connCh <- authedConn:
	case <-rl.done:
		authedConn.Close()
	}
}

// Accept returns the next authenticated connection
func (rl *RealityListener) Accept() (net.Conn, error) {
	select {
	case conn := <-rl.connCh:
		return conn, nil
	case <-rl.done:
		return nil, fmt.Errorf("listener closed")
	}
}

// Close shuts down the listener
func (rl *RealityListener) Close() error {
	close(rl.done)
	return rl.inner.Close()
}

// Addr returns the listener address
func (rl *RealityListener) Addr() net.Addr {
	return rl.inner.Addr()
}
