package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// FailoverStrategy defines how failover selects the next path
type FailoverStrategy string

const (
	// StrategyPriority selects the path with the lowest priority number
	StrategyPriority FailoverStrategy = "priority"
	// StrategyRoundRobin cycles through available paths
	StrategyRoundRobin FailoverStrategy = "round_robin"
	// StrategyLatency selects the path with the lowest latency
	StrategyLatency FailoverStrategy = "latency"
)

// PathState represents the health state of a path
type PathState int

const (
	PathHealthy PathState = iota
	PathDegraded
	PathUnhealthy
	PathDisabled
)

func (ps PathState) String() string {
	switch ps {
	case PathHealthy:
		return "healthy"
	case PathDegraded:
		return "degraded"
	case PathUnhealthy:
		return "unhealthy"
	case PathDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

// PathInfo holds runtime information about a path
type PathInfo struct {
	Config       config.PathConfig
	State        PathState
	Latency      time.Duration
	LastCheck    time.Time
	LastError    error
	FailCount    int
	SuccessCount int
}

// FailoverConfig configures the failover behavior
type FailoverConfig struct {
	Strategy       FailoverStrategy
	HealthInterval time.Duration // How often to check path health
	FailThreshold  int           // Consecutive failures before marking unhealthy
	RecoverAfter   time.Duration // Time before retrying an unhealthy path
	MaxRetries     int           // Max retries per path before failover
}

// DefaultFailoverConfig returns sensible defaults
func DefaultFailoverConfig() FailoverConfig {
	return FailoverConfig{
		Strategy:       StrategyPriority,
		HealthInterval: 10 * time.Second,
		FailThreshold:  3,
		RecoverAfter:   30 * time.Second,
		MaxRetries:     2,
	}
}

// Failover manages automatic path switching when the active path fails
type Failover struct {
	cfg        FailoverConfig
	paths      []PathInfo
	activePath atomic.Int32
	log        *logger.Logger
	events     *EventBus
	mu         sync.RWMutex
	done       chan struct{}
	wg         sync.WaitGroup

	// Callbacks
	onSwitch func(from, to config.PathConfig) error
}

// NewFailover creates a new failover manager
func NewFailover(paths []config.PathConfig, cfg FailoverConfig, log *logger.Logger, events *EventBus) *Failover {
	pathInfos := make([]PathInfo, len(paths))
	for i, p := range paths {
		pathInfos[i] = PathInfo{
			Config:    p,
			State:     PathHealthy,
			LastCheck: time.Now(),
		}
	}

	// Sort by priority (lower number = higher priority)
	sortPathsByPriority(pathInfos)

	f := &Failover{
		cfg:    cfg,
		paths:  pathInfos,
		log:    log,
		events: events,
		done:   make(chan struct{}),
	}
	f.activePath.Store(0)

	return f
}

// sortPathsByPriority sorts paths by priority (lower = better)
func sortPathsByPriority(paths []PathInfo) {
	for i := 0; i < len(paths)-1; i++ {
		for j := i + 1; j < len(paths); j++ {
			if paths[j].Config.Priority < paths[i].Config.Priority {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}
}

// OnSwitch registers a callback for when the active path changes
// The callback should establish a new connection using the new path config
func (f *Failover) OnSwitch(fn func(from, to config.PathConfig) error) {
	f.onSwitch = fn
}

// Start begins the failover health monitoring loop
func (f *Failover) Start() {
	f.wg.Add(1)
	go f.healthLoop()
	f.log.Info("Failover: started with %d paths (strategy: %s)", len(f.paths), f.cfg.Strategy)
}

// Stop shuts down the failover manager
func (f *Failover) Stop() {
	close(f.done)
	f.wg.Wait()
}

// ActivePath returns the currently active path configuration
func (f *Failover) ActivePath() config.PathConfig {
	f.mu.RLock()
	defer f.mu.RUnlock()
	idx := int(f.activePath.Load())
	if idx >= 0 && idx < len(f.paths) {
		return f.paths[idx].Config
	}
	return config.PathConfig{}
}

// ActivePathInfo returns full info about the active path
func (f *Failover) ActivePathInfo() PathInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()
	idx := int(f.activePath.Load())
	if idx >= 0 && idx < len(f.paths) {
		return f.paths[idx]
	}
	return PathInfo{}
}

// AllPaths returns info about all paths
func (f *Failover) AllPaths() []PathInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]PathInfo, len(f.paths))
	copy(result, f.paths)
	return result
}

// ReportFailure reports a connection failure on the active path
// Returns true if failover was triggered
func (f *Failover) ReportFailure(err error) bool {
	f.mu.Lock()
	idx := int(f.activePath.Load())
	if idx < 0 || idx >= len(f.paths) {
		f.mu.Unlock()
		return false
	}

	f.paths[idx].FailCount++
	f.paths[idx].LastError = err
	f.paths[idx].SuccessCount = 0

	shouldFailover := f.paths[idx].FailCount >= f.cfg.FailThreshold
	f.mu.Unlock()

	if shouldFailover {
		f.log.Warn("Failover: path %q exceeded fail threshold (%d failures)",
			f.paths[idx].Config.Name, f.paths[idx].FailCount)
		return f.doFailover(idx)
	}

	return false
}

// ReportSuccess reports a successful operation on the active path
func (f *Failover) ReportSuccess() {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := int(f.activePath.Load())
	if idx >= 0 && idx < len(f.paths) {
		f.paths[idx].SuccessCount++
		f.paths[idx].FailCount = 0
		f.paths[idx].State = PathHealthy
	}
}

// doFailover switches to the next available healthy path
func (f *Failover) doFailover(failedIdx int) bool {
	f.mu.Lock()

	// Mark current path as unhealthy
	f.paths[failedIdx].State = PathUnhealthy
	f.paths[failedIdx].LastCheck = time.Now()

	// Find next healthy path
	nextIdx := f.selectNextPath(failedIdx)
	if nextIdx < 0 {
		f.mu.Unlock()
		f.log.Error("Failover: no healthy paths available!")
		f.events.Emit(EventError, fmt.Errorf("all paths unhealthy"))
		return false
	}

	fromPath := f.paths[failedIdx].Config
	toPath := f.paths[nextIdx].Config
	f.activePath.Store(int32(nextIdx))
	f.mu.Unlock()

	f.log.Info("Failover: switching from %q to %q", fromPath.Name, toPath.Name)
	f.events.Emit(EventPathChanged, map[string]string{
		"from": fromPath.Name,
		"to":   toPath.Name,
	})

	// Execute switch callback
	if f.onSwitch != nil {
		if err := f.onSwitch(fromPath, toPath); err != nil {
			f.log.Error("Failover: switch callback failed: %v", err)
			// Try next path
			return f.doFailover(nextIdx)
		}
	}

	return true
}

// selectNextPath finds the best available path (must be called with lock held)
func (f *Failover) selectNextPath(excludeIdx int) int {
	switch f.cfg.Strategy {
	case StrategyPriority:
		return f.selectByPriority(excludeIdx)
	case StrategyRoundRobin:
		return f.selectRoundRobin(excludeIdx)
	case StrategyLatency:
		return f.selectByLatency(excludeIdx)
	default:
		return f.selectByPriority(excludeIdx)
	}
}

func (f *Failover) selectByPriority(excludeIdx int) int {
	bestIdx := -1
	bestPriority := int(^uint(0) >> 1) // MaxInt

	for i, p := range f.paths {
		if i == excludeIdx {
			continue
		}
		if p.State == PathUnhealthy || p.State == PathDisabled {
			continue
		}
		if p.Config.Priority < bestPriority {
			bestPriority = p.Config.Priority
			bestIdx = i
		}
	}
	return bestIdx
}

func (f *Failover) selectRoundRobin(excludeIdx int) int {
	n := len(f.paths)
	for i := 1; i < n; i++ {
		idx := (excludeIdx + i) % n
		if f.paths[idx].State != PathUnhealthy && f.paths[idx].State != PathDisabled {
			return idx
		}
	}
	return -1
}

func (f *Failover) selectByLatency(excludeIdx int) int {
	bestIdx := -1
	var bestLatency time.Duration

	for i, p := range f.paths {
		if i == excludeIdx {
			continue
		}
		if p.State == PathUnhealthy || p.State == PathDisabled {
			continue
		}
		if bestIdx == -1 || p.Latency < bestLatency {
			bestLatency = p.Latency
			bestIdx = i
		}
	}
	return bestIdx
}

// healthLoop periodically checks path health and recovers unhealthy paths
func (f *Failover) healthLoop() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.cfg.HealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.done:
			return
		case <-ticker.C:
			f.checkPaths()
		}
	}
}

// checkPaths evaluates all paths and attempts recovery of unhealthy ones
func (f *Failover) checkPaths() {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	activeIdx := int(f.activePath.Load())
	bestRecoveredIdx := -1

	for i := range f.paths {
		if f.paths[i].State == PathDisabled {
			continue
		}

		// Try to recover unhealthy paths after RecoverAfter duration
		if f.paths[i].State == PathUnhealthy {
			if now.Sub(f.paths[i].LastCheck) >= f.cfg.RecoverAfter {
				f.paths[i].State = PathDegraded
				f.paths[i].FailCount = 0
				f.paths[i].LastCheck = now
				f.log.Info("Failover: path %q marked for recovery probe", f.paths[i].Config.Name)

				// If this recovered path has higher priority than active, note it
				if f.paths[i].Config.Priority < f.paths[activeIdx].Config.Priority {
					bestRecoveredIdx = i
				}
			}
		}
	}

	// If a higher-priority path recovered, switch back to it
	if bestRecoveredIdx >= 0 && f.cfg.Strategy == StrategyPriority {
		fromPath := f.paths[activeIdx].Config
		toPath := f.paths[bestRecoveredIdx].Config
		f.activePath.Store(int32(bestRecoveredIdx))
		f.log.Info("Failover: recovering to higher-priority path %q", toPath.Name)
		f.events.Emit(EventPathChanged, map[string]string{
			"from": fromPath.Name,
			"to":   toPath.Name,
		})

		// Execute switch callback (unlock first to avoid deadlock)
		if f.onSwitch != nil {
			go func() {
				if err := f.onSwitch(fromPath, toPath); err != nil {
					f.log.Error("Failover: recovery switch failed: %v", err)
				}
			}()
		}
	}
}

// ForceSwitch manually switches to a specific path by name
func (f *Failover) ForceSwitch(pathName string) error {
	f.mu.Lock()

	currentIdx := int(f.activePath.Load())
	targetIdx := -1

	for i, p := range f.paths {
		if p.Config.Name == pathName {
			targetIdx = i
			break
		}
	}

	if targetIdx < 0 {
		f.mu.Unlock()
		return fmt.Errorf("path %q not found", pathName)
	}

	if targetIdx == currentIdx {
		f.mu.Unlock()
		return nil
	}

	fromPath := f.paths[currentIdx].Config
	toPath := f.paths[targetIdx].Config
	f.activePath.Store(int32(targetIdx))
	f.mu.Unlock()

	f.log.Info("Failover: forced switch from %q to %q", fromPath.Name, toPath.Name)
	f.events.Emit(EventPathChanged, map[string]string{
		"from": fromPath.Name,
		"to":   toPath.Name,
	})

	if f.onSwitch != nil {
		return f.onSwitch(fromPath, toPath)
	}
	return nil
}

// DisablePath disables a path (won't be used for failover)
func (f *Failover) DisablePath(pathName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, p := range f.paths {
		if p.Config.Name == pathName {
			f.paths[i].State = PathDisabled
			f.log.Info("Failover: path %q disabled", pathName)
			return
		}
	}
}

// EnablePath re-enables a disabled path
func (f *Failover) EnablePath(pathName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, p := range f.paths {
		if p.Config.Name == pathName {
			f.paths[i].State = PathDegraded
			f.paths[i].FailCount = 0
			f.paths[i].LastCheck = time.Now()
			f.log.Info("Failover: path %q re-enabled", pathName)
			return
		}
	}
}
