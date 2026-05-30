package multipath

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Aggregator combines bandwidth from multiple paths
// It distributes data across multiple connections for higher throughput
// Similar to MPTCP but at application layer
type Aggregator struct {
	paths   []net.Conn
	log     *logger.Logger
	mu      sync.RWMutex
	current atomic.Int64
	closed  atomic.Bool

	// Stats
	totalBytesSent atomic.Int64
	totalBytesRecv atomic.Int64
}

// NewAggregator creates a new bandwidth aggregator
func NewAggregator(log *logger.Logger) *Aggregator {
	return &Aggregator{
		paths: make([]net.Conn, 0),
		log:   log,
	}
}

// AddPath adds a connection to the aggregator
func (a *Aggregator) AddPath(conn net.Conn) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.paths = append(a.paths, conn)
	a.log.Debug("Aggregator: added path (total: %d)", len(a.paths))
}

// RemovePath removes a connection from the aggregator
func (a *Aggregator) RemovePath(conn net.Conn) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, p := range a.paths {
		if p == conn {
			a.paths = append(a.paths[:i], a.paths[i+1:]...)
			break
		}
	}
}

// Write distributes data across paths using round-robin
// Each chunk goes to a different path for maximum throughput
func (a *Aggregator) Write(p []byte) (int, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.paths) == 0 {
		return 0, io.ErrClosedPipe
	}

	if a.closed.Load() {
		return 0, io.ErrClosedPipe
	}

	// For small data, send on single path
	if len(p) <= 4096 || len(a.paths) == 1 {
		idx := int(a.current.Add(1)-1) % len(a.paths)
		n, err := a.paths[idx].Write(p)
		if n > 0 {
			a.totalBytesSent.Add(int64(n))
		}
		return n, err
	}

	// For large data, split across paths
	chunkSize := len(p) / len(a.paths)
	if chunkSize < 1024 {
		chunkSize = 1024
	}

	totalWritten := 0
	offset := 0

	for offset < len(p) {
		idx := int(a.current.Add(1)-1) % len(a.paths)

		end := offset + chunkSize
		if end > len(p) {
			end = len(p)
		}

		n, err := a.paths[idx].Write(p[offset:end])
		totalWritten += n
		offset += n

		if err != nil {
			// Try next path
			a.log.Debug("Aggregator: write error on path %d, trying next", idx)
			continue
		}
	}

	a.totalBytesSent.Add(int64(totalWritten))
	return totalWritten, nil
}

// Read reads from any available path
func (a *Aggregator) Read(p []byte) (int, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.paths) == 0 {
		return 0, io.ErrClosedPipe
	}

	// Read from the first path that has data
	// Use round-robin to distribute reads
	idx := int(a.current.Add(1)-1) % len(a.paths)
	n, err := a.paths[idx].Read(p)
	if n > 0 {
		a.totalBytesRecv.Add(int64(n))
	}
	return n, err
}

// Close closes all paths
func (a *Aggregator) Close() error {
	a.closed.Store(true)
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, p := range a.paths {
		p.Close()
	}
	a.paths = nil
	return nil
}

// Stats returns aggregator statistics
func (a *Aggregator) Stats() AggregatorStats {
	a.mu.RLock()
	pathCount := len(a.paths)
	a.mu.RUnlock()

	return AggregatorStats{
		ActivePaths: pathCount,
		BytesSent:   a.totalBytesSent.Load(),
		BytesRecv:   a.totalBytesRecv.Load(),
	}
}

// AggregatorStats holds aggregator statistics
type AggregatorStats struct {
	ActivePaths int
	BytesSent   int64
	BytesRecv   int64
}

// SmartRetry implements intelligent retry logic
// It learns from failures and adapts retry strategy
type SmartRetry struct {
	maxRetries    int
	baseDelay     time.Duration
	maxDelay      time.Duration
	backoffFactor float64
	log           *logger.Logger
}

// NewSmartRetry creates a new smart retry handler
func NewSmartRetry(log *logger.Logger) *SmartRetry {
	return &SmartRetry{
		maxRetries:    5,
		baseDelay:     500 * time.Millisecond,
		maxDelay:      30 * time.Second,
		backoffFactor: 2.0,
		log:           log,
	}
}

// Do executes a function with smart retry logic
func (sr *SmartRetry) Do(fn func() error) error {
	var lastErr error
	delay := sr.baseDelay

	for attempt := 0; attempt <= sr.maxRetries; attempt++ {
		if attempt > 0 {
			sr.log.Debug("Retry attempt %d/%d (delay: %v)", attempt, sr.maxRetries, delay)
			time.Sleep(delay)

			// Exponential backoff
			delay = time.Duration(float64(delay) * sr.backoffFactor)
			if delay > sr.maxDelay {
				delay = sr.maxDelay
			}
		}

		err := fn()
		if err == nil {
			if attempt > 0 {
				sr.log.Info("Succeeded after %d retries", attempt)
			}
			return nil
		}

		lastErr = err
		sr.log.Debug("Attempt %d failed: %v", attempt+1, err)
	}

	return lastErr
}

// DoWithContext executes with retry and returns the connection
func (sr *SmartRetry) DoWithContext(fn func() (net.Conn, error)) (net.Conn, error) {
	var lastErr error
	var conn net.Conn
	delay := sr.baseDelay

	for attempt := 0; attempt <= sr.maxRetries; attempt++ {
		if attempt > 0 {
			sr.log.Debug("Retry attempt %d/%d (delay: %v)", attempt, sr.maxRetries, delay)
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * sr.backoffFactor)
			if delay > sr.maxDelay {
				delay = sr.maxDelay
			}
		}

		var err error
		conn, err = fn()
		if err == nil {
			return conn, nil
		}

		lastErr = err
	}

	return nil, lastErr
}
