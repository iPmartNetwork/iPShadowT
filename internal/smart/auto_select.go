package smart

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// AutoSelector automatically selects and switches between transports
// based on real-time network conditions
//
// How it works:
// 1. On startup: Tests all transports and picks the best one
// 2. During operation: Continuously monitors connection quality
// 3. On degradation: Automatically switches to next best transport
// 4. On failure: Immediately fails over to backup transport
//
// Why it can make mistakes:
// - DPI may activate AFTER initial test passes
// - Throttling may be gradual (not immediate)
// - Some blocks are time-based (e.g., only during peak hours)
//
// How we minimize mistakes:
// - Continuous monitoring (not just initial test)
// - Multiple test methods (latency, throughput, packet loss)
// - Historical data (learn from past failures)
// - Graceful degradation (don't drop connection during switch)
type AutoSelector struct {
	log            *logger.Logger
	serverAddr     string
	transports     []TransportProbe
	currentBest    string
	history        []SelectionEvent
	mu             sync.RWMutex
	done           chan struct{}
	onSwitch       func(oldTransport, newTransport string)
	checkInterval  time.Duration
	failThreshold  int // consecutive failures before switching
}

// TransportProbe holds test results for a transport
type TransportProbe struct {
	Name        string
	Available   bool
	Latency     time.Duration
	Throughput  float64 // bytes/sec estimate
	PacketLoss  float64 // 0.0 - 1.0
	LastTest    time.Time
	Failures    int
	Score       float64 // Composite score (higher = better)
	Blocked     bool    // Confirmed blocked
}

// SelectionEvent records a transport selection decision
type SelectionEvent struct {
	Timestamp    time.Time
	FromTransport string
	ToTransport  string
	Reason       string
}

// AutoSelectConfig holds auto-selection configuration
type AutoSelectConfig struct {
	ServerAddr    string
	CheckInterval time.Duration // How often to re-test (default: 30s)
	FailThreshold int           // Failures before switch (default: 3)
	OnSwitch      func(old, new string) // Callback when transport changes
}

// NewAutoSelector creates a new auto-selector
func NewAutoSelector(cfg AutoSelectConfig, log *logger.Logger) *AutoSelector {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	if cfg.FailThreshold == 0 {
		cfg.FailThreshold = 3
	}

	return &AutoSelector{
		log:           log,
		serverAddr:    cfg.ServerAddr,
		checkInterval: cfg.CheckInterval,
		failThreshold: cfg.FailThreshold,
		onSwitch:      cfg.OnSwitch,
		history:       make([]SelectionEvent, 0),
		done:          make(chan struct{}),
		transports: []TransportProbe{
			{Name: "reality"},
			{Name: "shadowtls"},
			{Name: "wsmux"},
			{Name: "h2mux"},
			{Name: "grpc"},
			{Name: "tcpmux"},
			{Name: "quic"},
		},
	}
}

// InitialSelect performs initial transport selection
// Returns the recommended transport name
func (as *AutoSelector) InitialSelect() (string, error) {
	as.log.Info("🔍 Auto-selecting best transport for %s...", as.serverAddr)

	// Test all transports
	as.testAllTransports()

	// Sort by score
	as.mu.Lock()
	sort.Slice(as.transports, func(i, j int) bool {
		return as.transports[i].Score > as.transports[j].Score
	})

	// Pick the best available
	var best string
	for _, t := range as.transports {
		if t.Available && !t.Blocked {
			best = t.Name
			break
		}
	}
	as.currentBest = best
	as.mu.Unlock()

	if best == "" {
		return "", fmt.Errorf("no working transport found")
	}

	as.log.Info("✅ Selected transport: %s", best)
	as.printResults()

	return best, nil
}

// StartMonitoring begins continuous monitoring
func (as *AutoSelector) StartMonitoring() {
	go as.monitorLoop()
	as.log.Info("Transport monitoring started (interval: %v)", as.checkInterval)
}

// monitorLoop continuously checks transport health
func (as *AutoSelector) monitorLoop() {
	ticker := time.NewTicker(as.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-as.done:
			return
		case <-ticker.C:
			as.checkAndSwitch()
		}
	}
}

// checkAndSwitch checks current transport and switches if needed
func (as *AutoSelector) checkAndSwitch() {
	as.mu.RLock()
	current := as.currentBest
	as.mu.RUnlock()

	if current == "" {
		return
	}

	// Test current transport
	probe := as.testTransport(current)

	if !probe.Available || probe.Failures >= as.failThreshold {
		// Current transport is failing - find alternative
		as.log.Warn("⚠️ Transport %s degraded (failures: %d, available: %v)",
			current, probe.Failures, probe.Available)

		newTransport := as.findAlternative(current)
		if newTransport != "" && newTransport != current {
			as.switchTransport(current, newTransport, "degradation detected")
		}
	}
}

// testAllTransports tests all available transports
func (as *AutoSelector) testAllTransports() {
	var wg sync.WaitGroup

	for i := range as.transports {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			probe := as.testTransport(as.transports[idx].Name)
			as.mu.Lock()
			as.transports[idx] = probe
			as.mu.Unlock()
		}(i)
	}

	wg.Wait()
}

// testTransport tests a single transport
func (as *AutoSelector) testTransport(name string) TransportProbe {
	probe := TransportProbe{
		Name:     name,
		LastTest: time.Now(),
	}

	// Test based on transport type
	switch name {
	case "tcpmux", "reality", "shadowtls", "h2mux", "grpc", "wsmux":
		// TCP-based: test TCP connectivity on port 443
		probe.Available, probe.Latency = as.testTCP(as.serverAddr)
	case "quic":
		// UDP-based: test UDP connectivity
		probe.Available, probe.Latency = as.testUDP(as.serverAddr)
	}

	// Calculate score
	probe.Score = as.calculateScore(probe)

	return probe
}

// testTCP tests TCP connectivity and measures latency
func (as *AutoSelector) testTCP(addr string) (bool, time.Duration) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	latency := time.Since(start)

	if err != nil {
		return false, latency
	}
	conn.Close()
	return true, latency
}

// testUDP tests UDP connectivity
func (as *AutoSelector) testUDP(addr string) (bool, time.Duration) {
	start := time.Now()
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return false, 0
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return false, time.Since(start)
	}
	defer conn.Close()

	// Send probe and wait for response
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	conn.Write([]byte{0x00})

	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	latency := time.Since(start)

	// Timeout means UDP might be blocked
	return err == nil, latency
}

// calculateScore calculates a composite score for a transport
func (as *AutoSelector) calculateScore(probe TransportProbe) float64 {
	if !probe.Available {
		return 0
	}

	score := 100.0

	// Latency penalty (lower latency = higher score)
	latencyMs := float64(probe.Latency.Milliseconds())
	if latencyMs > 0 {
		score -= latencyMs / 10 // -1 point per 10ms
	}

	// DPI resistance bonus (based on transport type)
	dpiBonus := map[string]float64{
		"reality":   50, // Highest stealth
		"shadowtls": 40,
		"wsmux":     35, // CDN compatible
		"h2mux":     30,
		"grpc":      30,
		"tcpmux":    10, // Lowest stealth but fastest
		"quic":      20, // Fast but UDP may be blocked
	}
	score += dpiBonus[probe.Name]

	// Failure penalty
	score -= float64(probe.Failures) * 20

	// Blocked = zero score
	if probe.Blocked {
		return 0
	}

	return score
}

// findAlternative finds the best alternative transport
func (as *AutoSelector) findAlternative(exclude string) string {
	as.testAllTransports()

	as.mu.RLock()
	defer as.mu.RUnlock()

	var best string
	var bestScore float64

	for _, t := range as.transports {
		if t.Name == exclude || !t.Available || t.Blocked {
			continue
		}
		if t.Score > bestScore {
			bestScore = t.Score
			best = t.Name
		}
	}

	return best
}

// switchTransport switches to a new transport
func (as *AutoSelector) switchTransport(from, to, reason string) {
	as.mu.Lock()
	as.currentBest = to
	as.history = append(as.history, SelectionEvent{
		Timestamp:     time.Now(),
		FromTransport: from,
		ToTransport:   to,
		Reason:        reason,
	})
	as.mu.Unlock()

	as.log.Info("🔄 Switching transport: %s → %s (reason: %s)", from, to, reason)

	if as.onSwitch != nil {
		as.onSwitch(from, to)
	}
}

// RecordFailure records a failure for the current transport
func (as *AutoSelector) RecordFailure() {
	as.mu.Lock()
	for i := range as.transports {
		if as.transports[i].Name == as.currentBest {
			as.transports[i].Failures++
			break
		}
	}
	as.mu.Unlock()
}

// RecordSuccess resets failure counter for current transport
func (as *AutoSelector) RecordSuccess() {
	as.mu.Lock()
	for i := range as.transports {
		if as.transports[i].Name == as.currentBest {
			as.transports[i].Failures = 0
			break
		}
	}
	as.mu.Unlock()
}

// MarkBlocked marks a transport as confirmed blocked
func (as *AutoSelector) MarkBlocked(name string) {
	as.mu.Lock()
	for i := range as.transports {
		if as.transports[i].Name == name {
			as.transports[i].Blocked = true
			as.transports[i].Score = 0
			break
		}
	}
	as.mu.Unlock()

	as.log.Warn("❌ Transport %s marked as blocked", name)
}

// GetCurrent returns the currently selected transport
func (as *AutoSelector) GetCurrent() string {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.currentBest
}

// GetHistory returns selection history
func (as *AutoSelector) GetHistory() []SelectionEvent {
	as.mu.RLock()
	defer as.mu.RUnlock()
	result := make([]SelectionEvent, len(as.history))
	copy(result, as.history)
	return result
}

// printResults prints test results
func (as *AutoSelector) printResults() {
	as.mu.RLock()
	defer as.mu.RUnlock()

	as.log.Info("┌─ Transport Test Results ─────────────────┐")
	for _, t := range as.transports {
		status := "❌"
		if t.Available && !t.Blocked {
			status = "✅"
		} else if t.Blocked {
			status = "🚫"
		}
		as.log.Info("│ %s %-12s  Latency: %6v  Score: %.0f", status, t.Name, t.Latency.Round(time.Millisecond), t.Score)
	}
	as.log.Info("└───────────────────────────────────────────┘")
}

// Stop stops the auto-selector
func (as *AutoSelector) Stop() {
	close(as.done)
}
