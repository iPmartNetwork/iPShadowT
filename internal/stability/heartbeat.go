package stability

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Heartbeat provides application-level heartbeat monitoring for sessions
// Unlike TCP keepalive or smux keepalive, this measures actual RTT
// and detects dead sessions faster
type Heartbeat struct {
	log           *logger.Logger
	interval      time.Duration
	timeout       time.Duration
	maxMissed     int
	onDead        func(sessionID int)
	sessions      sync.Map // sessionID → *sessionHealth
	done          chan struct{}
	wg            sync.WaitGroup
}

type sessionHealth struct {
	lastPong    atomic.Int64 // unix nano of last pong received
	lastPing    atomic.Int64 // unix nano of last ping sent
	rtt         atomic.Int64 // last RTT in nanoseconds
	avgRTT      atomic.Int64 // exponential moving average RTT
	missed      atomic.Int32 // consecutive missed pongs
	alive       atomic.Bool
	pingFunc    func() error          // sends ping to remote
	pongChan    chan struct{}          // receives pong signal
}

// HeartbeatConfig configures the heartbeat monitor
type HeartbeatConfig struct {
	Interval  time.Duration // How often to ping (default: 5s)
	Timeout   time.Duration // Max time to wait for pong (default: 10s)
	MaxMissed int           // Max missed pongs before declaring dead (default: 3)
	OnDead    func(sessionID int) // Callback when session is dead
}

// NewHeartbeat creates a new heartbeat monitor
func NewHeartbeat(cfg HeartbeatConfig, log *logger.Logger) *Heartbeat {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxMissed == 0 {
		cfg.MaxMissed = 3
	}

	return &Heartbeat{
		log:       log,
		interval:  cfg.Interval,
		timeout:   cfg.Timeout,
		maxMissed: cfg.MaxMissed,
		onDead:    cfg.OnDead,
		done:      make(chan struct{}),
	}
}

// Register registers a session for heartbeat monitoring
func (h *Heartbeat) Register(sessionID int, pingFunc func() error) chan struct{} {
	sh := &sessionHealth{
		pingFunc: pingFunc,
		pongChan: make(chan struct{}, 1),
	}
	sh.alive.Store(true)
	sh.lastPong.Store(time.Now().UnixNano())
	h.sessions.Store(sessionID, sh)
	return sh.pongChan
}

// Unregister removes a session from monitoring
func (h *Heartbeat) Unregister(sessionID int) {
	h.sessions.Delete(sessionID)
}

// RecordPong records a pong response for a session
func (h *Heartbeat) RecordPong(sessionID int) {
	val, ok := h.sessions.Load(sessionID)
	if !ok {
		return
	}
	sh := val.(*sessionHealth)

	now := time.Now().UnixNano()
	pingTime := sh.lastPing.Load()
	if pingTime > 0 {
		rtt := now - pingTime
		sh.rtt.Store(rtt)

		// Exponential moving average (alpha = 0.3)
		oldAvg := sh.avgRTT.Load()
		if oldAvg == 0 {
			sh.avgRTT.Store(rtt)
		} else {
			newAvg := int64(float64(oldAvg)*0.7 + float64(rtt)*0.3)
			sh.avgRTT.Store(newAvg)
		}
	}

	sh.lastPong.Store(now)
	sh.missed.Store(0)
	sh.alive.Store(true)

	// Signal pong channel
	select {
	case sh.pongChan <- struct{}{}:
	default:
	}
}

// GetRTT returns the current RTT for a session in milliseconds
func (h *Heartbeat) GetRTT(sessionID int) time.Duration {
	val, ok := h.sessions.Load(sessionID)
	if !ok {
		return 0
	}
	sh := val.(*sessionHealth)
	return time.Duration(sh.avgRTT.Load())
}

// IsAlive returns whether a session is considered alive
func (h *Heartbeat) IsAlive(sessionID int) bool {
	val, ok := h.sessions.Load(sessionID)
	if !ok {
		return false
	}
	return val.(*sessionHealth).alive.Load()
}

// Start begins the heartbeat monitoring loop
func (h *Heartbeat) Start() {
	h.wg.Add(1)
	go h.monitorLoop()
	h.log.Info("Heartbeat monitor started (interval=%v, timeout=%v, maxMissed=%d)",
		h.interval, h.timeout, h.maxMissed)
}

// monitorLoop periodically pings all sessions
func (h *Heartbeat) monitorLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			h.pingAll()
		}
	}
}

// pingAll sends ping to all registered sessions
func (h *Heartbeat) pingAll() {
	h.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(int)
		sh := value.(*sessionHealth)

		// Record ping time
		sh.lastPing.Store(time.Now().UnixNano())

		// Send ping
		if sh.pingFunc != nil {
			if err := sh.pingFunc(); err != nil {
				sh.missed.Add(1)
				missed := int(sh.missed.Load())

				if missed >= h.maxMissed {
					sh.alive.Store(false)
					h.log.Warn("Session %d: dead (missed %d pongs)", sessionID, missed)
					if h.onDead != nil {
						go h.onDead(sessionID)
					}
				} else {
					h.log.Debug("Session %d: ping failed (missed: %d/%d)", sessionID, missed, h.maxMissed)
				}
			}
		}

		// Check timeout (even if ping succeeded, check if pong came back)
		lastPong := sh.lastPong.Load()
		elapsed := time.Since(time.Unix(0, lastPong))
		if elapsed > h.timeout*time.Duration(h.maxMissed) {
			sh.alive.Store(false)
			h.log.Warn("Session %d: timeout (no pong for %v)", sessionID, elapsed)
			if h.onDead != nil {
				go h.onDead(sessionID)
			}
		}

		return true
	})
}

// Stop stops the heartbeat monitor
func (h *Heartbeat) Stop() {
	close(h.done)
	h.wg.Wait()
}

// Stats returns heartbeat statistics for all sessions
func (h *Heartbeat) Stats() map[int]SessionStats {
	stats := make(map[int]SessionStats)
	h.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(int)
		sh := value.(*sessionHealth)
		stats[sessionID] = SessionStats{
			Alive:  sh.alive.Load(),
			RTT:    time.Duration(sh.rtt.Load()),
			AvgRTT: time.Duration(sh.avgRTT.Load()),
			Missed: int(sh.missed.Load()),
		}
		return true
	})
	return stats
}

// SessionStats holds stats for a single session
type SessionStats struct {
	Alive  bool
	RTT    time.Duration
	AvgRTT time.Duration
	Missed int
}
