package pool

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// AdaptivePool manages connections with dynamic sizing based on load
type AdaptivePool struct {
	log         *logger.Logger
	conns       chan net.Conn
	factory     func() (net.Conn, error)
	mu          sync.RWMutex
	minSize     int
	maxSize     int
	currentSize atomic.Int32
	activeConns atomic.Int32
	totalDials  atomic.Int64
	totalErrors atomic.Int64
	done        chan struct{}
	wg          sync.WaitGroup

	// Adaptive parameters
	scaleUpThreshold  float64 // Usage ratio to trigger scale up
	scaleDownThreshold float64 // Usage ratio to trigger scale down
	warmupCount       int     // Pre-connect count on startup
}

// PoolConfig configures the adaptive pool
type PoolConfig struct {
	MinSize            int
	MaxSize            int
	WarmupCount        int     // Pre-connect on startup (0 = min)
	ScaleUpThreshold   float64 // 0.8 = scale up when 80% used
	ScaleDownThreshold float64 // 0.2 = scale down when 20% used
	HealthCheckInterval time.Duration
	IdleTimeout        time.Duration
}

// DefaultPoolConfig returns sensible defaults
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MinSize:            4,
		MaxSize:            32,
		WarmupCount:        0,
		ScaleUpThreshold:   0.75,
		ScaleDownThreshold: 0.25,
		HealthCheckInterval: 30 * time.Second,
		IdleTimeout:        90 * time.Second,
	}
}

// NewAdaptivePool creates a new adaptive connection pool
func NewAdaptivePool(cfg PoolConfig, factory func() (net.Conn, error), log *logger.Logger) *AdaptivePool {
	if cfg.MinSize <= 0 {
		cfg.MinSize = 4
	}
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 32
	}
	if cfg.MaxSize < cfg.MinSize {
		cfg.MaxSize = cfg.MinSize
	}
	if cfg.ScaleUpThreshold <= 0 {
		cfg.ScaleUpThreshold = 0.75
	}
	if cfg.ScaleDownThreshold <= 0 {
		cfg.ScaleDownThreshold = 0.25
	}
	if cfg.WarmupCount <= 0 {
		cfg.WarmupCount = cfg.MinSize
	}

	p := &AdaptivePool{
		log:                log,
		conns:              make(chan net.Conn, cfg.MaxSize),
		factory:            factory,
		minSize:            cfg.MinSize,
		maxSize:            cfg.MaxSize,
		scaleUpThreshold:   cfg.ScaleUpThreshold,
		scaleDownThreshold: cfg.ScaleDownThreshold,
		warmupCount:        cfg.WarmupCount,
		done:               make(chan struct{}),
	}

	return p
}

// Start initializes the pool with warmup connections
func (p *AdaptivePool) Start() error {
	p.log.Info("Pool: warming up %d connections (min=%d, max=%d)", p.warmupCount, p.minSize, p.maxSize)

	// Warmup: pre-connect
	var wg sync.WaitGroup
	for i := 0; i < p.warmupCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := p.factory()
			if err != nil {
				p.totalErrors.Add(1)
				p.log.Debug("Pool warmup dial failed: %v", err)
				return
			}
			p.totalDials.Add(1)
			p.currentSize.Add(1)
			select {
			case p.conns <- conn:
			default:
				conn.Close()
				p.currentSize.Add(-1)
			}
		}()
	}
	wg.Wait()

	p.log.Info("Pool: ready with %d connections", p.currentSize.Load())

	// Start adaptive scaling loop
	p.wg.Add(1)
	go p.adaptiveLoop()

	return nil
}

// Get retrieves a connection from the pool or creates a new one
func (p *AdaptivePool) Get() (net.Conn, error) {
	// Try to get from pool first (non-blocking)
	select {
	case conn := <-p.conns:
		p.activeConns.Add(1)
		return &pooledConn{Conn: conn, pool: p}, nil
	default:
	}

	// Pool empty - create new connection
	if int(p.currentSize.Load()) < p.maxSize {
		conn, err := p.factory()
		if err != nil {
			p.totalErrors.Add(1)
			return nil, err
		}
		p.totalDials.Add(1)
		p.currentSize.Add(1)
		p.activeConns.Add(1)
		return &pooledConn{Conn: conn, pool: p}, nil
	}

	// At max capacity - wait for a connection
	select {
	case conn := <-p.conns:
		p.activeConns.Add(1)
		return &pooledConn{Conn: conn, pool: p}, nil
	case <-time.After(10 * time.Second):
		return nil, ErrPoolExhausted
	}
}

// put returns a connection to the pool
func (p *AdaptivePool) put(conn net.Conn) {
	p.activeConns.Add(-1)

	select {
	case p.conns <- conn:
		// Returned to pool
	default:
		// Pool full, close connection
		conn.Close()
		p.currentSize.Add(-1)
	}
}

// discard removes a bad connection from the pool
func (p *AdaptivePool) discard(conn net.Conn) {
	p.activeConns.Add(-1)
	p.currentSize.Add(-1)
	conn.Close()
}

// adaptiveLoop monitors pool usage and scales up/down
func (p *AdaptivePool) adaptiveLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.adjustSize()
		}
	}
}

// adjustSize scales the pool based on usage
func (p *AdaptivePool) adjustSize() {
	current := int(p.currentSize.Load())
	active := int(p.activeConns.Load())

	if current == 0 {
		return
	}

	usage := float64(active) / float64(current)

	// Scale up: high usage
	if usage >= p.scaleUpThreshold && current < p.maxSize {
		toAdd := min(current/2+1, p.maxSize-current)
		p.log.Debug("Pool: scaling up +%d (usage: %.0f%%, current: %d)", toAdd, usage*100, current)

		for i := 0; i < toAdd; i++ {
			go func() {
				conn, err := p.factory()
				if err != nil {
					p.totalErrors.Add(1)
					return
				}
				p.totalDials.Add(1)
				p.currentSize.Add(1)
				select {
				case p.conns <- conn:
				default:
					conn.Close()
					p.currentSize.Add(-1)
				}
			}()
		}
	}

	// Scale down: low usage (but not below min)
	if usage <= p.scaleDownThreshold && current > p.minSize {
		toRemove := min((current-p.minSize)/2+1, current-p.minSize)
		p.log.Debug("Pool: scaling down -%d (usage: %.0f%%, current: %d)", toRemove, usage*100, current)

		for i := 0; i < toRemove; i++ {
			select {
			case conn := <-p.conns:
				conn.Close()
				p.currentSize.Add(-1)
			default:
				break
			}
		}
	}
}

// Stats returns pool statistics
func (p *AdaptivePool) Stats() PoolStats {
	return PoolStats{
		CurrentSize: int(p.currentSize.Load()),
		ActiveConns: int(p.activeConns.Load()),
		IdleConns:   len(p.conns),
		TotalDials:  p.totalDials.Load(),
		TotalErrors: p.totalErrors.Load(),
		MinSize:     p.minSize,
		MaxSize:     p.maxSize,
	}
}

// PoolStats holds pool statistics
type PoolStats struct {
	CurrentSize int   `json:"current_size"`
	ActiveConns int   `json:"active_conns"`
	IdleConns   int   `json:"idle_conns"`
	TotalDials  int64 `json:"total_dials"`
	TotalErrors int64 `json:"total_errors"`
	MinSize     int   `json:"min_size"`
	MaxSize     int   `json:"max_size"`
}

// Close shuts down the pool
func (p *AdaptivePool) Close() {
	close(p.done)
	p.wg.Wait()

	// Drain and close all connections
	close(p.conns)
	for conn := range p.conns {
		conn.Close()
	}
}

// pooledConn wraps a connection to return it to the pool on Close
type pooledConn struct {
	net.Conn
	pool   *AdaptivePool
	closed bool
}

func (c *pooledConn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	// Return to pool instead of closing
	c.pool.put(c.Conn)
	return nil
}

// MarkBad marks this connection as bad (won't be returned to pool)
func (c *pooledConn) MarkBad() {
	if c.closed {
		return
	}
	c.closed = true
	c.pool.discard(c.Conn)
}

// ErrPoolExhausted is returned when the pool is at max capacity
var ErrPoolExhausted = &PoolError{msg: "connection pool exhausted"}

// PoolError represents a pool error
type PoolError struct {
	msg string
}

func (e *PoolError) Error() string { return e.msg }

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
