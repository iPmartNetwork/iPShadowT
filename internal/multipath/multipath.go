package multipath

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/transport"
)

// Strategy defines how paths are selected
type Strategy string

const (
	// StrategyPriority uses paths in priority order (failover)
	StrategyPriority Strategy = "priority"
	// StrategyRoundRobin distributes across all paths
	StrategyRoundRobin Strategy = "roundrobin"
	// StrategyFastest uses the path with lowest latency
	StrategyFastest Strategy = "fastest"
	// StrategyAuto automatically selects the best strategy
	StrategyAuto Strategy = "auto"
)

// Path represents a single network path to the server
type Path struct {
	Name      string
	Config    config.PathConfig
	Transport transport.Transport
	Latency   time.Duration
	Available atomic.Bool
	Failures  atomic.Int64
	LastCheck time.Time
	mu        sync.Mutex
}

// Manager manages multiple paths with failover and load balancing
type Manager struct {
	paths    []*Path
	strategy Strategy
	log      *logger.Logger
	current  atomic.Int64
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewManager creates a new multi-path manager
func NewManager(paths []config.PathConfig, strategy string, cfg *config.Config, log *logger.Logger) (*Manager, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths configured")
	}

	mgr := &Manager{
		paths:    make([]*Path, 0, len(paths)),
		strategy: Strategy(strategy),
		log:      log,
		done:     make(chan struct{}),
	}

	if mgr.strategy == "" {
		mgr.strategy = StrategyAuto
	}

	// Sort paths by priority
	sort.Slice(paths, func(i, j int) bool {
		return paths[i].Priority < paths[j].Priority
	})

	// Create transports for each path
	for _, pathCfg := range paths {
		// Create a config copy with this path's settings
		pathFullCfg := *cfg
		pathFullCfg.RemoteAddr = pathCfg.RemoteAddr
		pathFullCfg.Transport = pathCfg.Transport

		tp, err := transport.NewTransport(&pathFullCfg, log)
		if err != nil {
			log.Warn("Failed to create transport for path %q: %v", pathCfg.Name, err)
			continue
		}

		path := &Path{
			Name:      pathCfg.Name,
			Config:    pathCfg,
			Transport: tp,
		}
		path.Available.Store(true)

		mgr.paths = append(mgr.paths, path)
		log.Info("  Path: %s (%s via %s, priority: %d)", pathCfg.Name, pathCfg.RemoteAddr, pathCfg.Transport, pathCfg.Priority)
	}

	if len(mgr.paths) == 0 {
		return nil, fmt.Errorf("no valid paths could be created")
	}

	return mgr, nil
}

// Start begins path monitoring
func (m *Manager) Start() {
	m.wg.Add(1)
	go m.monitorPaths()
}

// Dial connects using the best available path
func (m *Manager) Dial() (net.Conn, error) {
	switch m.strategy {
	case StrategyPriority:
		return m.dialPriority()
	case StrategyRoundRobin:
		return m.dialRoundRobin()
	case StrategyFastest:
		return m.dialFastest()
	default: // Auto
		return m.dialAuto()
	}
}

// dialPriority tries paths in priority order
func (m *Manager) dialPriority() (net.Conn, error) {
	var lastErr error

	for _, path := range m.paths {
		if !path.Available.Load() {
			continue
		}

		conn, err := path.Transport.Dial()
		if err != nil {
			lastErr = err
			path.Failures.Add(1)
			m.log.Warn("Path %q failed: %v", path.Name, err)
			continue
		}

		path.Failures.Store(0)
		m.log.Debug("Connected via path: %s", path.Name)
		return conn, nil
	}

	return nil, fmt.Errorf("all paths failed, last error: %w", lastErr)
}

// dialRoundRobin distributes connections across paths
func (m *Manager) dialRoundRobin() (net.Conn, error) {
	attempts := len(m.paths)
	var lastErr error

	for i := 0; i < attempts; i++ {
		idx := int(m.current.Add(1)-1) % len(m.paths)
		path := m.paths[idx]

		if !path.Available.Load() {
			continue
		}

		conn, err := path.Transport.Dial()
		if err != nil {
			lastErr = err
			path.Failures.Add(1)
			continue
		}

		path.Failures.Store(0)
		return conn, nil
	}

	return nil, fmt.Errorf("all paths failed: %w", lastErr)
}

// dialFastest connects using the path with lowest latency
func (m *Manager) dialFastest() (net.Conn, error) {
	// Sort by latency
	available := make([]*Path, 0)
	for _, p := range m.paths {
		if p.Available.Load() {
			available = append(available, p)
		}
	}

	if len(available) == 0 {
		return nil, fmt.Errorf("no paths available")
	}

	sort.Slice(available, func(i, j int) bool {
		return available[i].Latency < available[j].Latency
	})

	// Try fastest first
	var lastErr error
	for _, path := range available {
		conn, err := path.Transport.Dial()
		if err != nil {
			lastErr = err
			path.Failures.Add(1)
			continue
		}
		path.Failures.Store(0)
		return conn, nil
	}

	return nil, fmt.Errorf("all paths failed: %w", lastErr)
}

// dialAuto uses priority for first attempt, then fastest
func (m *Manager) dialAuto() (net.Conn, error) {
	// First try priority-based
	conn, err := m.dialPriority()
	if err == nil {
		return conn, nil
	}

	// If all priority paths failed, try any available
	for _, path := range m.paths {
		path.Available.Store(true) // Reset and retry
	}

	return m.dialFastest()
}

// monitorPaths periodically checks path health
func (m *Manager) monitorPaths() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.checkPaths()
		}
	}
}

// checkPaths probes each path for availability and latency
func (m *Manager) checkPaths() {
	for _, path := range m.paths {
		start := time.Now()

		conn, err := path.Transport.Dial()
		if err != nil {
			path.Available.Store(false)
			m.log.Debug("Path %q: unavailable (%v)", path.Name, err)
			continue
		}

		latency := time.Since(start)
		conn.Close()

		path.mu.Lock()
		path.Latency = latency
		path.LastCheck = time.Now()
		path.mu.Unlock()

		path.Available.Store(true)
		path.Failures.Store(0)

		m.log.Debug("Path %q: available (latency: %v)", path.Name, latency)
	}
}

// GetStatus returns the status of all paths
func (m *Manager) GetStatus() []PathStatus {
	statuses := make([]PathStatus, 0, len(m.paths))

	for _, path := range m.paths {
		path.mu.Lock()
		status := PathStatus{
			Name:      path.Name,
			Transport: path.Config.Transport,
			Remote:    path.Config.RemoteAddr,
			Priority:  path.Config.Priority,
			Available: path.Available.Load(),
			Latency:   path.Latency,
			Failures:  path.Failures.Load(),
			LastCheck: path.LastCheck,
		}
		path.mu.Unlock()
		statuses = append(statuses, status)
	}

	return statuses
}

// PathStatus represents the current status of a path
type PathStatus struct {
	Name      string
	Transport string
	Remote    string
	Priority  int
	Available bool
	Latency   time.Duration
	Failures  int64
	LastCheck time.Time
}

// Close shuts down the manager
func (m *Manager) Close() {
	close(m.done)
	m.wg.Wait()

	for _, path := range m.paths {
		path.Transport.Close()
	}
}
