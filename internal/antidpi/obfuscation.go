package antidpi

import (
	"crypto/rand"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// ObfuscationMode defines the traffic obfuscation strategy
type ObfuscationMode string

const (
	// ModeMimicHTTPS makes traffic look like normal HTTPS browsing
	ModeMimicHTTPS ObfuscationMode = "mimic_https"
	// ModeMimicVideo makes traffic look like video streaming
	ModeMimicVideo ObfuscationMode = "mimic_video"
	// ModeRandomBurst sends data in random bursts
	ModeRandomBurst ObfuscationMode = "random_burst"
	// ModeConstantRate sends at a constant rate with padding
	ModeConstantRate ObfuscationMode = "constant_rate"
)

// ObfuscationConfig configures traffic obfuscation
type ObfuscationConfig struct {
	Mode           ObfuscationMode
	Enabled        bool
	PaddingEnabled bool
	MinDelay       time.Duration // Minimum inter-packet delay
	MaxDelay       time.Duration // Maximum inter-packet delay
	TargetRate     int64         // Target bytes/sec for constant rate mode
}

// DefaultObfuscationConfig returns defaults
func DefaultObfuscationConfig() ObfuscationConfig {
	return ObfuscationConfig{
		Mode:           ModeMimicHTTPS,
		Enabled:        true,
		PaddingEnabled: true,
		MinDelay:       0,
		MaxDelay:       5 * time.Millisecond,
		TargetRate:     0,
	}
}

// Obfuscator wraps a connection with traffic obfuscation
type Obfuscator struct {
	conn   net.Conn
	cfg    ObfuscationConfig
	log    *logger.Logger
	mu     sync.Mutex
	closed bool

	// HTTPS mimicry state
	httpsState *httpsMimicState
	// Video mimicry state
	videoState *videoMimicState
}

// httpsMimicState tracks HTTPS browsing pattern
type httpsMimicState struct {
	// Typical HTTPS patterns: small request, large response, idle, repeat
	phase       int // 0=request, 1=response, 2=idle
	phaseBytes  int
	targetBytes int
}

// videoMimicState tracks video streaming pattern
type videoMimicState struct {
	// Video: periodic large chunks (segments) with small keep-alive between
	segmentSize  int
	segmentsSent int
	lastSegment  time.Time
}

// NewObfuscator creates a new traffic obfuscator
func NewObfuscator(conn net.Conn, cfg ObfuscationConfig, log *logger.Logger) *Obfuscator {
	o := &Obfuscator{
		conn: conn,
		cfg:  cfg,
		log:  log,
	}

	switch cfg.Mode {
	case ModeMimicHTTPS:
		o.httpsState = &httpsMimicState{
			phase:       0,
			targetBytes: randomRange(200, 800), // Initial request size
		}
	case ModeMimicVideo:
		o.videoState = &videoMimicState{
			segmentSize: randomRange(65536, 262144), // 64KB-256KB segments
			lastSegment: time.Now(),
		}
	}

	return o
}

// Write sends data with obfuscation applied
func (o *Obfuscator) Write(p []byte) (int, error) {
	if !o.cfg.Enabled {
		return o.conn.Write(p)
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	switch o.cfg.Mode {
	case ModeMimicHTTPS:
		return o.writeHTTPSMimic(p)
	case ModeMimicVideo:
		return o.writeVideoMimic(p)
	case ModeRandomBurst:
		return o.writeRandomBurst(p)
	case ModeConstantRate:
		return o.writeConstantRate(p)
	default:
		return o.conn.Write(p)
	}
}

// writeHTTPSMimic shapes traffic to look like HTTPS browsing
func (o *Obfuscator) writeHTTPSMimic(p []byte) (int, error) {
	totalWritten := 0
	remaining := p

	for len(remaining) > 0 {
		// Determine chunk size based on current phase
		var chunkSize int
		switch o.httpsState.phase {
		case 0: // Request phase: small packets (100-1500 bytes)
			chunkSize = randomRange(100, 1500)
		case 1: // Response phase: larger packets (1400-16384 bytes)
			chunkSize = randomRange(1400, 16384)
		case 2: // Idle phase: tiny keep-alive
			chunkSize = randomRange(20, 100)
		}

		if chunkSize > len(remaining) {
			chunkSize = len(remaining)
		}

		// Add random padding if enabled
		chunk := remaining[:chunkSize]
		if o.cfg.PaddingEnabled && chunkSize < 1400 {
			padSize := randomRange(0, 100)
			padded := make([]byte, chunkSize+padSize)
			copy(padded, chunk)
			rand.Read(padded[chunkSize:])
			chunk = padded
		}

		n, err := o.conn.Write(chunk[:chunkSize])
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}

		remaining = remaining[chunkSize:]
		o.httpsState.phaseBytes += chunkSize

		// Phase transition
		if o.httpsState.phaseBytes >= o.httpsState.targetBytes {
			o.httpsState.phase = (o.httpsState.phase + 1) % 3
			o.httpsState.phaseBytes = 0
			switch o.httpsState.phase {
			case 0:
				o.httpsState.targetBytes = randomRange(200, 800)
			case 1:
				o.httpsState.targetBytes = randomRange(5000, 50000)
			case 2:
				o.httpsState.targetBytes = randomRange(50, 200)
				// Add idle delay
				time.Sleep(time.Duration(randomRange(10, 100)) * time.Millisecond)
			}
		}

		// Inter-packet delay
		if o.cfg.MaxDelay > 0 {
			delay := randomDuration(o.cfg.MinDelay, o.cfg.MaxDelay)
			time.Sleep(delay)
		}
	}

	return totalWritten, nil
}

// writeVideoMimic shapes traffic to look like video streaming
func (o *Obfuscator) writeVideoMimic(p []byte) (int, error) {
	totalWritten := 0
	remaining := p

	for len(remaining) > 0 {
		// Video streaming: send in segment-sized bursts
		chunkSize := o.videoState.segmentSize
		if chunkSize > len(remaining) {
			chunkSize = len(remaining)
		}

		// Simulate segment download timing
		elapsed := time.Since(o.videoState.lastSegment)
		segmentInterval := time.Duration(randomRange(1000, 4000)) * time.Millisecond
		if elapsed < segmentInterval {
			time.Sleep(segmentInterval - elapsed)
		}

		n, err := o.conn.Write(remaining[:chunkSize])
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}

		remaining = remaining[chunkSize:]
		o.videoState.segmentsSent++
		o.videoState.lastSegment = time.Now()

		// Vary segment size slightly
		o.videoState.segmentSize = randomRange(65536, 262144)
	}

	return totalWritten, nil
}

// writeRandomBurst sends data in random-sized bursts with random delays
func (o *Obfuscator) writeRandomBurst(p []byte) (int, error) {
	totalWritten := 0
	remaining := p

	for len(remaining) > 0 {
		// Random burst size
		burstSize := randomRange(512, 8192)
		if burstSize > len(remaining) {
			burstSize = len(remaining)
		}

		n, err := o.conn.Write(remaining[:burstSize])
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}

		remaining = remaining[burstSize:]

		// Random delay between bursts
		delay := time.Duration(randomRange(1, 20)) * time.Millisecond
		time.Sleep(delay)
	}

	return totalWritten, nil
}

// writeConstantRate sends at a constant rate, padding if needed
func (o *Obfuscator) writeConstantRate(p []byte) (int, error) {
	if o.cfg.TargetRate <= 0 {
		return o.conn.Write(p)
	}

	totalWritten := 0
	remaining := p

	// Calculate bytes per tick (10ms intervals)
	bytesPerTick := o.cfg.TargetRate / 100

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for len(remaining) > 0 {
		<-ticker.C

		chunkSize := int(bytesPerTick)
		if chunkSize > len(remaining) {
			chunkSize = len(remaining)
		}

		n, err := o.conn.Write(remaining[:chunkSize])
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}

		remaining = remaining[chunkSize:]
	}

	return totalWritten, nil
}

// Read reads from the underlying connection
func (o *Obfuscator) Read(p []byte) (int, error) {
	return o.conn.Read(p)
}

// Close closes the obfuscator and underlying connection
func (o *Obfuscator) Close() error {
	o.closed = true
	return o.conn.Close()
}

// WrapConn wraps a net.Conn with obfuscation
func WrapWithObfuscation(conn net.Conn, cfg ObfuscationConfig, log *logger.Logger) io.ReadWriteCloser {
	return NewObfuscator(conn, cfg, log)
}

// Helper functions
func randomRange(min, max int) int {
	if min >= max {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	return int(n.Int64()) + min
}

func randomDuration(min, max time.Duration) time.Duration {
	if min >= max {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	return time.Duration(n.Int64()) + min
}
