package loadbalancer

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Strategy defines load balancing strategy
type Strategy string

const (
	StrategyRoundRobin   Strategy = "roundrobin"
	StrategyLeastConn    Strategy = "leastconn"
	StrategyWeighted     Strategy = "weighted"
	StrategyIPHash       Strategy = "iphash"
	StrategyFastest      Strategy = "fastest"
)

// Backend represents a backend server
type Backend struct {
	Address     string
	Weight      int
	Healthy     atomic.Bool
	Connections atomic.Int64
	Latency     atomic.Int64 // microseconds
	TotalReqs   atomic.Int64
	FailCount   atomic.Int64
}

// Balancer distributes traffic across multiple backend servers
type Balancer struct {
	backends []*Backend
	strategy Strategy
	current  atomic.Int64
	log      *logger.Logger
	mu       sync.RWMutex
	done     chan struct{}
}

// NewBalancer creates a new load balancer
func NewBalancer(strategy Strategy, log *logger.Logger) *Balancer {
	return &Balancer{
		backends: make([]*Backend, 0),
		strategy: strategy,
		log:      log,
		done:     make(chan struct{}),
	}
}

// AddBackend adds a backend server
func (b *Balancer) AddBackend(address string, weight int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	backend := &Backend{
		Address: address,
		Weight:  weight,
	}
	backend.Healthy.Store(true)

	b.backends = append(b.backends, backend)
	b.log.Info("Load balancer: added backend %s (weight: %d)", address, weight)
}

// GetBackend returns the next backend based on strategy
func (b *Balancer) GetBackend(clientIP string) (*Backend, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	healthy := b.getHealthy()
	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy backends available")
	}

	switch b.strategy {
	case StrategyRoundRobin:
		return b.roundRobin(healthy), nil
	case StrategyLeastConn:
		return b.leastConn(healthy), nil
	case StrategyWeighted:
		return b.weighted(healthy), nil
	case StrategyIPHash:
		return b.ipHash(healthy, clientIP), nil
	case StrategyFastest:
		return b.fastest(healthy), nil
	default:
		return b.roundRobin(healthy), nil
	}
}

func (b *Balancer) roundRobin(backends []*Backend) *Backend {
	idx := int(b.current.Add(1)-1) % len(backends)
	return backends[idx]
}

func (b *Balancer) leastConn(backends []*Backend) *Backend {
	var best *Backend
	var minConn int64 = 1<<63 - 1

	for _, backend := range backends {
		conns := backend.Connections.Load()
		if conns < minConn {
			minConn = conns
			best = backend
		}
	}
	return best
}

func (b *Balancer) weighted(backends []*Backend) *Backend {
	totalWeight := 0
	for _, backend := range backends {
		totalWeight += backend.Weight
	}

	idx := int(b.current.Add(1)-1) % totalWeight
	for _, backend := range backends {
		idx -= backend.Weight
		if idx < 0 {
			return backend
		}
	}
	return backends[0]
}

func (b *Balancer) ipHash(backends []*Backend, clientIP string) *Backend {
	hash := 0
	for _, c := range clientIP {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return backends[hash%len(backends)]
}

func (b *Balancer) fastest(backends []*Backend) *Backend {
	var best *Backend
	var minLatency int64 = 1<<63 - 1

	for _, backend := range backends {
		lat := backend.Latency.Load()
		if lat < minLatency {
			minLatency = lat
			best = backend
		}
	}
	return best
}

func (b *Balancer) getHealthy() []*Backend {
	healthy := make([]*Backend, 0)
	for _, backend := range b.backends {
		if backend.Healthy.Load() {
			healthy = append(healthy, backend)
		}
	}
	return healthy
}

// StartHealthCheck begins periodic health checking
func (b *Balancer) StartHealthCheck(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-b.done:
				return
			case <-ticker.C:
				b.checkHealth()
			}
		}
	}()
}

func (b *Balancer) checkHealth() {
	b.mu.RLock()
	backends := b.backends
	b.mu.RUnlock()

	for _, backend := range backends {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", backend.Address, 5*time.Second)
		latency := time.Since(start)

		if err != nil {
			backend.Healthy.Store(false)
			backend.FailCount.Add(1)
			b.log.Debug("Backend %s: unhealthy (%v)", backend.Address, err)
		} else {
			conn.Close()
			backend.Healthy.Store(true)
			backend.Latency.Store(latency.Microseconds())
			backend.FailCount.Store(0)
		}
	}
}

// Close shuts down the balancer
func (b *Balancer) Close() {
	close(b.done)
}
