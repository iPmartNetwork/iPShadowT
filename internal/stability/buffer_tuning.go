package stability

import (
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// BufferTuner automatically adjusts buffer sizes based on measured BDP
// BDP (Bandwidth-Delay Product) = Bandwidth × RTT
// Optimal buffer size should be >= BDP for full throughput
type BufferTuner struct {
	log            *logger.Logger
	currentRecv    atomic.Int64
	currentSend    atomic.Int64
	measuredBDP    atomic.Int64
	bandwidth      atomic.Int64 // bytes/sec
	rtt            atomic.Int64 // nanoseconds
	minBuffer      int
	maxBuffer      int
}

// BufferConfig configures buffer tuning
type BufferConfig struct {
	MinBuffer int // Minimum buffer size (default: 64KB)
	MaxBuffer int // Maximum buffer size (default: 16MB)
}

// NewBufferTuner creates a new buffer tuner
func NewBufferTuner(cfg BufferConfig, log *logger.Logger) *BufferTuner {
	if cfg.MinBuffer == 0 {
		cfg.MinBuffer = 65536 // 64KB
	}
	if cfg.MaxBuffer == 0 {
		cfg.MaxBuffer = 16777216 // 16MB
	}

	bt := &BufferTuner{
		log:       log,
		minBuffer: cfg.MinBuffer,
		maxBuffer: cfg.MaxBuffer,
	}
	bt.currentRecv.Store(int64(cfg.MinBuffer))
	bt.currentSend.Store(int64(cfg.MinBuffer))

	return bt
}

// UpdateMetrics updates the measured bandwidth and RTT
func (bt *BufferTuner) UpdateMetrics(bandwidth int64, rtt time.Duration) {
	bt.bandwidth.Store(bandwidth)
	bt.rtt.Store(int64(rtt))

	// Calculate BDP
	bdp := int64(float64(bandwidth) * rtt.Seconds())
	bt.measuredBDP.Store(bdp)

	// Optimal buffer = 2 × BDP (for safety margin)
	optimalBuffer := bdp * 2
	if optimalBuffer < int64(bt.minBuffer) {
		optimalBuffer = int64(bt.minBuffer)
	}
	if optimalBuffer > int64(bt.maxBuffer) {
		optimalBuffer = int64(bt.maxBuffer)
	}

	oldRecv := bt.currentRecv.Load()
	bt.currentRecv.Store(optimalBuffer)
	bt.currentSend.Store(optimalBuffer)

	if optimalBuffer != oldRecv {
		bt.log.Debug("Buffer tuned: %d → %d (BDP=%d, BW=%d B/s, RTT=%v)",
			oldRecv, optimalBuffer, bdp, bandwidth, rtt)
	}
}

// GetRecvBuffer returns the recommended receive buffer size
func (bt *BufferTuner) GetRecvBuffer() int {
	return int(bt.currentRecv.Load())
}

// GetSendBuffer returns the recommended send buffer size
func (bt *BufferTuner) GetSendBuffer() int {
	return int(bt.currentSend.Load())
}

// GetBDP returns the measured Bandwidth-Delay Product
func (bt *BufferTuner) GetBDP() int64 {
	return bt.measuredBDP.Load()
}

// WarmupPool keeps pre-connected sessions ready for immediate use
// This eliminates connection setup latency for new streams
type WarmupPool struct {
	log         *logger.Logger
	pool        chan interface{} // buffered channel of ready sessions
	factory     func() (interface{}, error)
	minReady    int
	maxReady    int
	done        chan struct{}
}

// WarmupConfig configures the warmup pool
type WarmupConfig struct {
	MinReady int                          // Minimum pre-connected sessions (default: 2)
	MaxReady int                          // Maximum pre-connected sessions (default: 4)
	Factory  func() (interface{}, error)  // Creates a new session
}

// NewWarmupPool creates a new warmup pool
func NewWarmupPool(cfg WarmupConfig, log *logger.Logger) *WarmupPool {
	if cfg.MinReady == 0 {
		cfg.MinReady = 2
	}
	if cfg.MaxReady == 0 {
		cfg.MaxReady = 4
	}

	wp := &WarmupPool{
		log:      log,
		pool:     make(chan interface{}, cfg.MaxReady),
		factory:  cfg.Factory,
		minReady: cfg.MinReady,
		maxReady: cfg.MaxReady,
		done:     make(chan struct{}),
	}

	return wp
}

// Start begins maintaining the warmup pool
func (wp *WarmupPool) Start() {
	// Initial warmup
	go wp.fill()

	// Maintenance loop
	go wp.maintainLoop()

	wp.log.Info("Warmup pool started (min=%d, max=%d)", wp.minReady, wp.maxReady)
}

// Get retrieves a pre-connected session (or creates one if pool empty)
func (wp *WarmupPool) Get() (interface{}, error) {
	select {
	case session := <-wp.pool:
		// Got a pre-connected session — refill in background
		go wp.fillOne()
		return session, nil
	default:
		// Pool empty — create on demand
		return wp.factory()
	}
}

// fill fills the pool to minReady
func (wp *WarmupPool) fill() {
	for len(wp.pool) < wp.minReady {
		session, err := wp.factory()
		if err != nil {
			wp.log.Debug("Warmup: pre-connect failed: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		select {
		case wp.pool <- session:
		default:
			// Pool full
			return
		}
	}
}

// fillOne adds one session to the pool
func (wp *WarmupPool) fillOne() {
	if len(wp.pool) >= wp.maxReady {
		return
	}
	session, err := wp.factory()
	if err != nil {
		return
	}
	select {
	case wp.pool <- session:
	default:
	}
}

// maintainLoop keeps the pool filled
func (wp *WarmupPool) maintainLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wp.done:
			return
		case <-ticker.C:
			if len(wp.pool) < wp.minReady {
				wp.fill()
			}
		}
	}
}

// Count returns the number of ready sessions
func (wp *WarmupPool) Count() int {
	return len(wp.pool)
}

// Stop stops the warmup pool
func (wp *WarmupPool) Stop() {
	close(wp.done)
	// Drain pool
	close(wp.pool)
}
