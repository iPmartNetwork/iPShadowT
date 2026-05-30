package ratelimit

import (
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Limiter provides per-user rate limiting
type Limiter struct {
	buckets sync.Map // map[string]*Bucket
	log     *logger.Logger
}

// Bucket represents a token bucket for rate limiting
type Bucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// Config holds rate limiter configuration
type Config struct {
	// MaxBytesPerSecond is the maximum bytes per second per user (0 = unlimited)
	MaxBytesPerSecond int64
	// BurstSize is the maximum burst size in bytes
	BurstSize int64
}

// NewLimiter creates a new rate limiter
func NewLimiter(log *logger.Logger) *Limiter {
	return &Limiter{log: log}
}

// SetLimit sets the rate limit for a user
func (l *Limiter) SetLimit(userID string, bytesPerSecond int64, burstSize int64) {
	if bytesPerSecond <= 0 {
		l.buckets.Delete(userID)
		return
	}

	if burstSize <= 0 {
		burstSize = bytesPerSecond * 2 // Default burst = 2 seconds worth
	}

	bucket := &Bucket{
		tokens:     float64(burstSize),
		maxTokens:  float64(burstSize),
		refillRate: float64(bytesPerSecond),
		lastRefill: time.Now(),
	}

	l.buckets.Store(userID, bucket)
}

// Allow checks if n bytes can be sent/received for a user
// Returns the number of bytes allowed (may be less than requested)
func (l *Limiter) Allow(userID string, n int) int {
	val, ok := l.buckets.Load(userID)
	if !ok {
		return n // No limit
	}

	bucket := val.(*Bucket)
	return bucket.take(n)
}

// take removes tokens from the bucket
func (b *Bucket) take(n int) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now

	// Take tokens
	if b.tokens <= 0 {
		return 0
	}

	allowed := n
	if float64(allowed) > b.tokens {
		allowed = int(b.tokens)
	}

	b.tokens -= float64(allowed)
	return allowed
}

// WrapConn wraps a connection with rate limiting
func (l *Limiter) WrapConn(conn net.Conn, userID string) net.Conn {
	_, ok := l.buckets.Load(userID)
	if !ok {
		return conn // No limit
	}

	return &RateLimitedConn{
		Conn:    conn,
		limiter: l,
		userID:  userID,
	}
}

// RateLimitedConn wraps a net.Conn with rate limiting
type RateLimitedConn struct {
	net.Conn
	limiter *Limiter
	userID  string
}

// Read applies rate limiting to reads
func (c *RateLimitedConn) Read(p []byte) (int, error) {
	allowed := c.limiter.Allow(c.userID, len(p))
	if allowed == 0 {
		// Wait a bit and retry
		time.Sleep(10 * time.Millisecond)
		allowed = c.limiter.Allow(c.userID, len(p))
		if allowed == 0 {
			allowed = 1 // Always allow at least 1 byte to prevent deadlock
		}
	}

	if allowed < len(p) {
		p = p[:allowed]
	}

	return c.Conn.Read(p)
}

// Write applies rate limiting to writes
func (c *RateLimitedConn) Write(p []byte) (int, error) {
	totalWritten := 0

	for totalWritten < len(p) {
		remaining := len(p) - totalWritten
		allowed := c.limiter.Allow(c.userID, remaining)

		if allowed == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		n, err := c.Conn.Write(p[totalWritten : totalWritten+allowed])
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}
	}

	return totalWritten, nil
}
