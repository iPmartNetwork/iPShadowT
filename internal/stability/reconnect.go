package stability

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Reconnector handles graceful reconnection with exponential backoff
// It ensures:
// - New session is established BEFORE old one is closed
// - Active streams are not interrupted during reconnect
// - Backoff prevents hammering the server
// - Transport switching on repeated failures
type Reconnector struct {
	log            *logger.Logger
	cfg            ReconnectConfig
	attempts       atomic.Int32
	lastAttempt    atomic.Int64
	state          atomic.Int32 // 0=idle, 1=reconnecting, 2=cooldown
	mu             sync.Mutex
	connectFunc    func() error
	switchFunc     func() error // switch transport
	done           chan struct{}
}

// ReconnectState represents the reconnector state
type ReconnectState int32

const (
	StateIdle         ReconnectState = 0
	StateReconnecting ReconnectState = 1
	StateCooldown     ReconnectState = 2
)

// ReconnectConfig configures the reconnector
type ReconnectConfig struct {
	InitialDelay    time.Duration // First retry delay (default: 100ms)
	MaxDelay        time.Duration // Maximum backoff delay (default: 30s)
	BackoffFactor   float64       // Multiplier per attempt (default: 2.0)
	MaxAttempts     int           // Max attempts before switching transport (default: 5)
	ResetAfter      time.Duration // Reset attempt counter after this idle time (default: 60s)
	JitterPercent   float64       // Random jitter 0.0-1.0 (default: 0.2)
}

// DefaultReconnectConfig returns sensible defaults
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		MaxAttempts:   5,
		ResetAfter:    60 * time.Second,
		JitterPercent: 0.2,
	}
}

// NewReconnector creates a new reconnector
func NewReconnector(cfg ReconnectConfig, log *logger.Logger) *Reconnector {
	return &Reconnector{
		log:  log,
		cfg:  cfg,
		done: make(chan struct{}),
	}
}

// SetConnectFunc sets the function to call for reconnection
func (r *Reconnector) SetConnectFunc(fn func() error) {
	r.connectFunc = fn
}

// SetSwitchFunc sets the function to call when max attempts reached (transport switch)
func (r *Reconnector) SetSwitchFunc(fn func() error) {
	r.switchFunc = fn
}

// Trigger initiates a reconnection attempt
// Returns immediately if already reconnecting
func (r *Reconnector) Trigger() {
	if !r.state.CompareAndSwap(int32(StateIdle), int32(StateReconnecting)) {
		return // Already reconnecting
	}

	go r.reconnectLoop()
}

// reconnectLoop performs reconnection with exponential backoff
func (r *Reconnector) reconnectLoop() {
	defer r.state.Store(int32(StateIdle))

	// Check if we should reset attempt counter
	lastAttempt := time.Unix(0, r.lastAttempt.Load())
	if time.Since(lastAttempt) > r.cfg.ResetAfter {
		r.attempts.Store(0)
	}

	for {
		select {
		case <-r.done:
			return
		default:
		}

		attempt := int(r.attempts.Add(1))
		r.lastAttempt.Store(time.Now().UnixNano())

		// Check if we should switch transport
		if attempt > r.cfg.MaxAttempts {
			r.log.Warn("Reconnect: max attempts (%d) reached, switching transport", r.cfg.MaxAttempts)
			if r.switchFunc != nil {
				if err := r.switchFunc(); err != nil {
					r.log.Error("Reconnect: transport switch failed: %v", err)
				} else {
					r.attempts.Store(0) // Reset on successful switch
					return
				}
			}
		}

		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)
		r.log.Info("Reconnect: attempt %d/%d (delay: %v)", attempt, r.cfg.MaxAttempts, delay)

		// Wait
		select {
		case <-r.done:
			return
		case <-time.After(delay):
		}

		// Attempt reconnection
		if r.connectFunc != nil {
			if err := r.connectFunc(); err != nil {
				r.log.Warn("Reconnect: attempt %d failed: %v", attempt, err)
				continue
			}
		}

		// Success
		r.log.Info("Reconnect: succeeded after %d attempts", attempt)
		r.attempts.Store(0)
		return
	}
}

// calculateDelay calculates the backoff delay for a given attempt
func (r *Reconnector) calculateDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return r.cfg.InitialDelay
	}

	// Exponential backoff
	delay := float64(r.cfg.InitialDelay) * math.Pow(r.cfg.BackoffFactor, float64(attempt-1))

	// Cap at max delay
	if delay > float64(r.cfg.MaxDelay) {
		delay = float64(r.cfg.MaxDelay)
	}

	// Add jitter
	if r.cfg.JitterPercent > 0 {
		jitter := delay * r.cfg.JitterPercent
		// Simple deterministic jitter based on attempt number
		delay += jitter * float64(attempt%3-1) / 2
	}

	return time.Duration(delay)
}

// GetState returns the current reconnector state
func (r *Reconnector) GetState() ReconnectState {
	return ReconnectState(r.state.Load())
}

// GetAttempts returns the current attempt count
func (r *Reconnector) GetAttempts() int {
	return int(r.attempts.Load())
}

// Reset resets the attempt counter
func (r *Reconnector) Reset() {
	r.attempts.Store(0)
}

// Stop stops the reconnector
func (r *Reconnector) Stop() {
	close(r.done)
}
