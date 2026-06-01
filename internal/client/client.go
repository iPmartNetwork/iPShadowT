package client

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/antidpi"
	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/crypto"
	"github.com/iPmart/iPShadowT/internal/dns"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/multipath"
	"github.com/iPmart/iPShadowT/internal/mux"
	"github.com/iPmart/iPShadowT/internal/stability"
	"github.com/iPmart/iPShadowT/internal/transport"
	"github.com/iPmart/iPShadowT/internal/tunnel"
	"github.com/iPmart/iPShadowT/internal/utils"
)

// Client manages the tunnel connection to the server
type Client struct {
	cfg         *config.Config
	log         *logger.Logger
	transport   transport.Transport
	paths       *multipath.Manager
	encryptor   *crypto.Encryptor
	pool        *mux.SessionPool
	forwards    []*tunnel.Forwarder
	done        chan struct{}
	wg          sync.WaitGroup
	dnsResolver *dns.Resolver
	obfuscation *antidpi.ObfuscationConfig

	// Stability modules
	heartbeat   *stability.Heartbeat
	reconnector *stability.Reconnector
	quality     *stability.QualityMonitor
	dpiDetect   *stability.DPIDetector
	bufferTuner *stability.BufferTuner
	warmup      *stability.WarmupPool

	// Direct pool (when mux disabled)
	directPool  chan net.Conn
	poolSize    int
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

	c := &Client{
		cfg:       cfg,
		log:       log,
		transport: tp,
		encryptor: enc,
		pool:      mux.NewSessionPool(&cfg.Mux, log),
		done:      make(chan struct{}),
	}

	if len(cfg.Paths) > 0 {
		pm, err := multipath.NewManager(cfg.Paths, "priority", cfg, log)
		if err != nil {
			return nil, fmt.Errorf("multipath: %w", err)
		}
		c.paths = pm
		pm.Start()
		log.Info("Multi-path enabled: %d paths", len(cfg.Paths))
	}

	// Initialize DNS-over-HTTPS resolver to prevent DNS leaks
	c.dnsResolver = dns.NewResolver(nil, log)
	log.Info("DNS leak protection: enabled (DoH)")

	// Initialize traffic obfuscation config
	if cfg.AntiDPI.Enabled && cfg.AntiDPI.TrafficShape {
		obfCfg := antidpi.DefaultObfuscationConfig()
		c.obfuscation = &obfCfg
		log.Info("Traffic obfuscation: enabled (mode: %s)", obfCfg.Mode)
	}

	// Initialize stability modules
	c.heartbeat = stability.NewHeartbeat(stability.HeartbeatConfig{
		Interval:  5 * time.Second,
		Timeout:   10 * time.Second,
		MaxMissed: 3,
		OnDead: func(sessionID int) {
			log.Warn("Session %d declared dead by heartbeat", sessionID)
			c.quality.Unregister(sessionID)
			c.reconnector.Trigger()
		},
	}, log)

	c.quality = stability.NewQualityMonitor(stability.QualityConfig{
		CheckInterval: 5 * time.Second,
		MaxLatency:    500 * time.Millisecond,
		MaxJitter:     200 * time.Millisecond,
		MaxPacketLoss: 0.1,
		DegradeCallback: func(sessionID int, reason string) {
			log.Warn("Session %d quality degraded: %s", sessionID, reason)
		},
	}, log)

	c.reconnector = stability.NewReconnector(stability.DefaultReconnectConfig(), log)
	c.reconnector.SetConnectFunc(func() error {
		session, err := c.createSession()
		if err != nil {
			return err
		}
		c.pool.Add(session)
		return nil
	})
	c.reconnector.SetSwitchFunc(func() error {
		// Get DPI recommendation for best transport
		recommended := c.dpiDetect.GetRecommendation()
		if recommended != "" && recommended != c.cfg.Transport {
			log.Warn("Switching transport: %s → %s (DPI recommendation)", c.cfg.Transport, recommended)
			c.cfg.Transport = recommended
			// Recreate transport
			newTP, err := transport.NewTransport(c.cfg, log)
			if err != nil {
				return fmt.Errorf("transport switch failed: %w", err)
			}
			c.transport.Close()
			c.transport = newTP
			// Try connecting with new transport
			session, err := c.createSession()
			if err != nil {
				return err
			}
			c.pool.Add(session)
			return nil
		}
		return fmt.Errorf("no alternative transport available")
	})

	c.dpiDetect = stability.NewDPIDetector(log, func(pattern stability.DPIPattern) {
		recommended := c.dpiDetect.GetRecommendation()
		log.Warn("DPI pattern %s detected, recommended transport: %s", pattern, recommended)
	})

	maxBuf := cfg.Performance.RecvBuffer * 4
	if maxBuf <= 0 {
		maxBuf = 67108864
	}
	c.bufferTuner = stability.NewBufferTuner(stability.BufferConfig{
		MinBuffer: cfg.Performance.SendBuffer,
		MaxBuffer: maxBuf,
	}, log)
	if cfg.Performance.SendBuffer > 0 {
		c.bufferTuner.UpdateMetrics(int64(cfg.Performance.SendBuffer), 100*time.Millisecond)
	}

	// Initialize warmup pool (pre-connects sessions in background)
	c.warmup = stability.NewWarmupPool(stability.WarmupConfig{
		MinReady: 1,
		MaxReady: 2,
		Factory: func() (interface{}, error) {
			return c.createSession()
		},
	}, log)

	return c, nil
}

// Start connects to the server and starts port forwarding
func (c *Client) Start() error {
	if c.cfg.Performance.KernelTuning {
		kt := utils.NewKernelTuning(c.log)
		if err := kt.Apply(c.cfg.Performance.BufferProfile); err != nil {
			c.log.Warn("Kernel tuning: %v", err)
		}
	}

	c.log.Info("Upload tuning: send_buf=%d recv_buf=%d mux_stream=%d profile=%s",
		c.cfg.Performance.SendBuffer, c.cfg.Performance.RecvBuffer,
		c.cfg.Mux.StreamBuffer, c.cfg.Performance.BufferProfile)

	// Resolve remote address using DoH (prevents DNS poisoning)
	resolvedAddr, err := c.dnsResolver.ResolveAddr(c.cfg.RemoteAddr)
	if err != nil {
		c.log.Warn("DoH resolve failed for %s, using original: %v", c.cfg.RemoteAddr, err)
	} else if resolvedAddr != c.cfg.RemoteAddr {
		c.log.Info("DNS resolved: %s → %s (via DoH)", c.cfg.RemoteAddr, resolvedAddr)
	}

	// Establish mux sessions (skip if mux disabled)
	if c.cfg.Mux.Enabled {
		if err := c.connectSessions(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	} else {
		c.log.Info("⚡ Direct relay mode (mux disabled) — pre-connecting pool...")
		// Pre-connect a pool of raw connections (handshake done once per connection)
		if err := c.initDirectPool(); err != nil {
			return fmt.Errorf("failed to init direct pool: %w", err)
		}
	}

	// Start port forwarders
	if err := c.startForwarders(); err != nil {
		return fmt.Errorf("failed to start forwarders: %w", err)
	}

	// Start session maintenance (only if mux enabled)
	if c.cfg.Mux.Enabled {
		c.wg.Add(1)
		go c.maintainSessions()

		if c.cfg.Heartbeat.Enabled {
			c.pool.SetQualityFunc(func(sessionID int) int {
				return c.quality.GetScore(sessionID)
			})
			c.heartbeat.Start()
			c.quality.Start()
		}
		c.warmup.Start()
	} else {
		// Maintain direct pool
		c.wg.Add(1)
		go c.maintainDirectPool()
	}

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

		// Register session with heartbeat monitor
		sessionID := connected
		if c.cfg.Heartbeat.Enabled {
			c.heartbeat.Register(sessionID, func() error {
				if session.IsClosed() {
					return fmt.Errorf("session closed")
				}
				start := time.Now()
				stream, err := session.OpenStream()
				if err != nil {
					c.quality.RecordPing(sessionID)
					return err
				}
				stream.Close()
				rtt := time.Since(start)
				c.quality.RecordRTT(sessionID, rtt)
				c.quality.RecordPing(sessionID)
				c.quality.RecordPong(sessionID)
				return nil
			})
			c.quality.Register(sessionID)
		}
	}

	if connected == 0 {
		return fmt.Errorf("failed to establish any session: %w", firstErr)
	}

	c.log.Info("✅ Connected with %d/%d mux sessions", connected, concurrency)
	return nil
}

// dialTransport opens a transport connection (multipath or primary).
func (c *Client) dialTransport() (net.Conn, error) {
	if c.paths != nil {
		return c.paths.Dial()
	}
	return c.transport.Dial()
}

// createSession creates a single mux session
func (c *Client) createSession() (*mux.Session, error) {
	conn, err := c.dialTransport()
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	// Apply traffic obfuscation if enabled
	var muxConn net.Conn = conn
	if c.obfuscation != nil {
		obfConn := antidpi.NewObfuscator(conn, *c.obfuscation, c.log)
		muxConn = &obfuscatedConn{Conn: conn, obf: obfConn}
	}

	// Apply SNI spoofing if enabled (only for TCP-based transports)
	if c.cfg.AntiDPI.Enabled && c.cfg.AntiDPI.SNISpoof {
		// SNI spoofing only works with TLS/TCP transports, not UDP-based ones
		isTCPTransport := c.cfg.Transport == "tcpmux" || c.cfg.Transport == "wsmux" ||
			c.cfg.Transport == "h2mux" || c.cfg.Transport == "grpc" ||
			c.cfg.Transport == "reality" || c.cfg.Transport == "shadowtls"

		if isTCPTransport {
			spoofCfg := antidpi.SNISpoofConfig{
				FakeSNI: c.cfg.AntiDPI.SNISpoofDomain,
				Method:  antidpi.SpoofMethod(c.cfg.AntiDPI.SNISpoofMethod),
			}
			if spoofCfg.FakeSNI == "" {
				spoofCfg.FakeSNI = "www.google.com"
			}
			if spoofCfg.Method == "" {
				spoofCfg.Method = antidpi.MethodSplit
			}
			spoofer := antidpi.NewSNISpoofing(spoofCfg, c.log)
			muxConn = spoofer.WrapConn(muxConn)
		}
	}

	// Perform handshake
	if err := c.handshake(muxConn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	// Apply buffer tuner recommendations to mux config
	muxCfg := c.cfg.Mux
	tunedRecv := c.bufferTuner.GetRecvBuffer()
	tunedSend := c.bufferTuner.GetSendBuffer()
	if tunedRecv > muxCfg.RecvBuffer {
		muxCfg.RecvBuffer = tunedRecv
	}
	if tunedSend > muxCfg.StreamBuffer {
		muxCfg.StreamBuffer = tunedSend
	}
	if c.cfg.Performance.SendBuffer > muxCfg.StreamBuffer {
		muxCfg.StreamBuffer = c.cfg.Performance.SendBuffer
	}
	if c.cfg.Performance.RecvBuffer > muxCfg.RecvBuffer {
		muxCfg.RecvBuffer = c.cfg.Performance.RecvBuffer
	}

	utils.OptimizeTCP(conn, c.cfg.Performance)

	// Create mux session with tuned buffers
	session, err := mux.NewClientSession(muxConn, &muxCfg, c.log)
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

		// If mux is disabled, use direct pool (pre-connected, no handshake per request)
		if !c.cfg.Mux.Enabled {
			c.log.Info("  ⚡ Direct mode — using pre-connected pool")
			fwd.SetDirectDial(func() (net.Conn, error) {
				return c.getDirectConn()
			})
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
		// Record failure for DPI detection
		c.dpiDetect.RecordFailure(c.cfg.Transport, fmt.Errorf("session closed unexpectedly"), 0)
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
			start := time.Now()

			// Try warmup pool first (instant, no dial latency)
			var session *mux.Session
			var err error
			warmSession, warmErr := c.warmup.Get()
			if warmErr == nil && warmSession != nil {
				session = warmSession.(*mux.Session)
				c.log.Debug("Got pre-connected session from warmup pool")
			} else {
				// Fallback: create new session
				session, err = c.createSession()
				if err != nil {
					c.log.Error("Reconnect failed: %v", err)
					c.dpiDetect.RecordFailure(c.cfg.Transport, err, time.Since(start))
					continue
				}
			}

			c.pool.Add(session)

			// Register with heartbeat
			newID := active + i + 1
			c.heartbeat.Register(newID, func() error {
				if session.IsClosed() {
					return fmt.Errorf("session closed")
				}
				start := time.Now()
				stream, err := session.OpenStream()
				if err != nil {
					c.quality.RecordPing(newID)
					return err
				}
				stream.Close()
				rtt := time.Since(start)
				c.quality.RecordRTT(newID, rtt)
				c.quality.RecordPing(newID)
				c.quality.RecordPong(newID)
				return nil
			})
			c.quality.Register(newID)

			// Update buffer tuner with connection RTT
			rtt := time.Since(start)
			c.bufferTuner.UpdateMetrics(1048576, rtt) // Assume 1MB/s initially
			c.log.Info("✅ Session reconnected (%d/%d, RTT: %v)", active+i+1, target, rtt)
		}

		// If still no sessions, trigger reconnector with backoff
		if c.pool.Count() == 0 {
			c.reconnector.Trigger()
		}
	}
}

// initDirectPool pre-connects a pool of raw TCP connections (handshake done once)
func (c *Client) initDirectPool() error {
	c.poolSize = c.cfg.Pool.Size
	if c.poolSize <= 0 {
		c.poolSize = 16
	}
	c.directPool = make(chan net.Conn, c.poolSize*2)

	c.log.Info("Pre-connecting %d direct connections...", c.poolSize)

	connected := 0
	for i := 0; i < c.poolSize; i++ {
		conn, err := c.dialAndHandshake()
		if err != nil {
			c.log.Warn("Direct pool connect %d failed: %v", i+1, err)
			continue
		}
		c.directPool <- conn
		connected++
	}

	if connected == 0 {
		return fmt.Errorf("failed to establish any direct connection")
	}

	c.log.Info("✅ Direct pool ready: %d/%d connections", connected, c.poolSize)
	return nil
}

// dialAndHandshake creates a new connection with handshake completed
func (c *Client) dialAndHandshake() (net.Conn, error) {
	conn, err := c.dialTransport()
	if err != nil {
		return nil, err
	}
	if err := c.handshake(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// getDirectConn gets a pre-connected connection from the pool
// If pool is empty, dials a new one on-demand
func (c *Client) getDirectConn() (net.Conn, error) {
	// Try to get from pool (non-blocking)
	select {
	case conn := <-c.directPool:
		// Got a pre-connected one — refill in background
		go c.refillPool(1)
		return conn, nil
	default:
		// Pool empty — dial on demand
		return c.dialAndHandshake()
	}
}

// refillPool adds connections back to the pool in background
func (c *Client) refillPool(count int) {
	for i := 0; i < count; i++ {
		conn, err := c.dialAndHandshake()
		if err != nil {
			continue
		}
		select {
		case c.directPool <- conn:
		default:
			// Pool full, close extra
			conn.Close()
		}
	}
}

// maintainDirectPool keeps the direct pool filled
func (c *Client) maintainDirectPool() {
	defer c.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			current := len(c.directPool)
			if current < c.poolSize/2 {
				needed := c.poolSize - current
				c.log.Debug("Direct pool: refilling %d connections (current: %d)", needed, current)
				c.refillPool(needed)
			}
		}
	}
}

// Stop gracefully shuts down the client
func (c *Client) Stop() {
	close(c.done)

	// Stop stability modules
	c.heartbeat.Stop()
	c.quality.Stop()
	c.reconnector.Stop()
	c.warmup.Stop()

	// Stop forwarders
	for _, fwd := range c.forwards {
		fwd.Stop()
	}

	// Close session pool
	c.pool.Close()

	// Drain direct pool
	if c.directPool != nil {
		close(c.directPool)
		for conn := range c.directPool {
			conn.Close()
		}
	}

	// Close transport
	c.transport.Close()
	if c.paths != nil {
		c.paths.Close()
	}

	c.wg.Wait()
	c.log.Info("Client stopped")
}

// obfuscatedConn wraps a net.Conn with traffic obfuscation on writes
type obfuscatedConn struct {
	net.Conn
	obf *antidpi.Obfuscator
}

func (c *obfuscatedConn) Write(p []byte) (int, error) {
	return c.obf.Write(p)
}

func (c *obfuscatedConn) Read(p []byte) (int, error) {
	return c.obf.Read(p)
}

func (c *obfuscatedConn) Close() error {
	c.obf.Close()
	return c.Conn.Close()
}
