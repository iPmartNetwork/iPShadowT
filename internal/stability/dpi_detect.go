package stability

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// DPIDetector passively detects DPI interference patterns
// by analyzing connection failure modes
type DPIDetector struct {
	log      *logger.Logger
	events   []DPIEvent
	mu       sync.Mutex
	patterns map[DPIPattern]int // pattern → occurrence count
	onDetect func(pattern DPIPattern)
	total    atomic.Int64
}

// DPIPattern represents a detected DPI behavior
type DPIPattern string

const (
	// PatternTCPReset — connection reset after handshake (active probe response)
	PatternTCPReset DPIPattern = "tcp_reset"
	// PatternTimeout — connection timeout (port blocking)
	PatternTimeout DPIPattern = "timeout"
	// PatternTLSBlock — TLS handshake interrupted (SNI filtering)
	PatternTLSBlock DPIPattern = "tls_block"
	// PatternThrottle — connection works but extremely slow (bandwidth throttling)
	PatternThrottle DPIPattern = "throttle"
	// PatternDataReset — reset after data transfer starts (pattern matching)
	PatternDataReset DPIPattern = "data_reset"
	// PatternDNSPoison — DNS returns wrong IP
	PatternDNSPoison DPIPattern = "dns_poison"
	// PatternNone — no DPI detected
	PatternNone DPIPattern = "none"
)

// DPIEvent records a single DPI-related event
type DPIEvent struct {
	Timestamp time.Time
	Pattern   DPIPattern
	Transport string
	Duration  time.Duration // How long before failure
	Details   string
}

// NewDPIDetector creates a new DPI detector
func NewDPIDetector(log *logger.Logger, onDetect func(DPIPattern)) *DPIDetector {
	return &DPIDetector{
		log:      log,
		events:   make([]DPIEvent, 0, 100),
		patterns: make(map[DPIPattern]int),
		onDetect: onDetect,
	}
}

// RecordFailure records a connection failure for DPI analysis
func (d *DPIDetector) RecordFailure(transport string, err error, connectDuration time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	pattern := d.classifyFailure(err, connectDuration)
	d.total.Add(1)

	event := DPIEvent{
		Timestamp: time.Now(),
		Pattern:   pattern,
		Transport: transport,
		Duration:  connectDuration,
		Details:   err.Error(),
	}

	d.events = append(d.events, event)
	if len(d.events) > 1000 {
		d.events = d.events[len(d.events)-500:]
	}

	d.patterns[pattern]++

	// Log significant patterns
	if d.patterns[pattern] >= 3 {
		d.log.Warn("DPI detected: %s (transport: %s, occurrences: %d)",
			pattern, transport, d.patterns[pattern])
		if d.onDetect != nil {
			go d.onDetect(pattern)
		}
	}
}

// classifyFailure determines the DPI pattern from failure characteristics
func (d *DPIDetector) classifyFailure(err error, duration time.Duration) DPIPattern {
	errStr := err.Error()

	// Immediate reset (< 1s) = active DPI
	if duration < 1*time.Second {
		if contains(errStr, "reset") || contains(errStr, "RST") {
			return PatternTCPReset
		}
	}

	// Timeout (> 10s) = port blocking
	if duration > 10*time.Second {
		if contains(errStr, "timeout") || contains(errStr, "deadline") {
			return PatternTimeout
		}
	}

	// TLS-specific errors
	if contains(errStr, "tls") || contains(errStr, "handshake") || contains(errStr, "certificate") {
		return PatternTLSBlock
	}

	// Reset after some data = pattern matching DPI
	if duration > 2*time.Second && duration < 10*time.Second {
		if contains(errStr, "reset") || contains(errStr, "broken pipe") || contains(errStr, "connection refused") {
			return PatternDataReset
		}
	}

	// DNS errors
	if contains(errStr, "no such host") || contains(errStr, "dns") {
		return PatternDNSPoison
	}

	return PatternNone
}

// GetRecommendation returns the recommended transport based on detected patterns
func (d *DPIDetector) GetRecommendation() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Find dominant pattern
	var dominant DPIPattern
	maxCount := 0
	for pattern, count := range d.patterns {
		if count > maxCount {
			maxCount = count
			dominant = pattern
		}
	}

	switch dominant {
	case PatternTCPReset:
		return "reality" // REALITY resists active probing
	case PatternTimeout:
		return "kcp" // UDP may bypass TCP blocking
	case PatternTLSBlock:
		return "shadowtls" // ShadowTLS uses real TLS handshake
	case PatternThrottle:
		return "wsmux" // CDN bypasses throttling
	case PatternDataReset:
		return "h2mux" // HTTP/2 looks like normal traffic
	case PatternDNSPoison:
		return "reality" // REALITY doesn't need DNS
	default:
		return "reality" // Default recommendation
	}
}

// GetStats returns DPI detection statistics
func (d *DPIDetector) GetStats() map[DPIPattern]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make(map[DPIPattern]int)
	for k, v := range d.patterns {
		result[k] = v
	}
	return result
}

// Reset clears all recorded events
func (d *DPIDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = d.events[:0]
	d.patterns = make(map[DPIPattern]int)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
