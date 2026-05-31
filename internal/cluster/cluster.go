package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Node represents a server node in the cluster
type Node struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`   // IP:Port
	Region    string    `json:"region"`    // Geographic region
	Weight    int       `json:"weight"`    // Load balancing weight
	Healthy   bool      `json:"healthy"`
	LastSeen  time.Time `json:"last_seen"`
	Latency   int64     `json:"latency_ms"` // Latency in ms
	Load      float64   `json:"load"`       // Current load (0.0-1.0)
	Capacity  int       `json:"capacity"`   // Max connections
	ActiveConns int     `json:"active_conns"`
}

// Cluster manages a group of iPShadowT server nodes
type Cluster struct {
	log       *logger.Logger
	nodes     map[string]*Node
	mu        sync.RWMutex
	localNode *Node
	done      chan struct{}
	wg        sync.WaitGroup
	apiAddr   string

	// Stats
	totalRequests atomic.Int64
	failovers     atomic.Int64
}

// ClusterConfig configures the cluster
type ClusterConfig struct {
	LocalID    string   // This node's ID
	LocalAddr  string   // This node's address
	Region     string   // This node's region
	Capacity   int      // Max connections for this node
	PeerAddrs  []string // Addresses of other nodes
	APIAddr    string   // Cluster API listen address
	SyncInterval time.Duration
}

// NewCluster creates a new cluster manager
func NewCluster(cfg ClusterConfig, log *logger.Logger) *Cluster {
	localNode := &Node{
		ID:       cfg.LocalID,
		Address:  cfg.LocalAddr,
		Region:   cfg.Region,
		Weight:   1,
		Healthy:  true,
		Capacity: cfg.Capacity,
		LastSeen: time.Now(),
	}

	c := &Cluster{
		log:       log,
		nodes:     make(map[string]*Node),
		localNode: localNode,
		done:      make(chan struct{}),
		apiAddr:   cfg.APIAddr,
	}

	// Add local node
	c.nodes[localNode.ID] = localNode

	// Add peer nodes
	for i, addr := range cfg.PeerAddrs {
		node := &Node{
			ID:      fmt.Sprintf("peer-%d", i),
			Address: addr,
			Weight:  1,
			Healthy: false, // Will be updated by health check
		}
		c.nodes[node.ID] = node
	}

	return c
}

// Start begins cluster operations
func (c *Cluster) Start() error {
	// Start cluster API
	if c.apiAddr != "" {
		go c.startAPI()
	}

	// Start health checking peers
	c.wg.Add(1)
	go c.healthCheckLoop()

	// Start state sync
	c.wg.Add(1)
	go c.syncLoop()

	c.log.Info("Cluster: started (local=%s, peers=%d)", c.localNode.ID, len(c.nodes)-1)
	return nil
}

// GetBestNode returns the best node for a new connection
// Uses geographic proximity and current load
func (c *Cluster) GetBestNode(clientRegion string) *Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var best *Node
	var bestScore float64

	for _, node := range c.nodes {
		if !node.Healthy {
			continue
		}

		score := c.calculateNodeScore(node, clientRegion)
		if best == nil || score > bestScore {
			best = node
			bestScore = score
		}
	}

	c.totalRequests.Add(1)
	return best
}

// calculateNodeScore scores a node for selection
func (c *Cluster) calculateNodeScore(node *Node, clientRegion string) float64 {
	score := 100.0

	// Load penalty (higher load = lower score)
	score -= node.Load * 50

	// Latency penalty
	if node.Latency > 0 {
		score -= float64(node.Latency) / 10
	}

	// Capacity bonus (more available capacity = higher score)
	if node.Capacity > 0 {
		available := float64(node.Capacity-node.ActiveConns) / float64(node.Capacity)
		score += available * 20
	}

	// Region bonus (same region = higher score)
	if clientRegion != "" && node.Region == clientRegion {
		score += 30
	}

	// Weight multiplier
	score *= float64(node.Weight)

	return score
}

// ReportNodeDown marks a node as unhealthy and triggers failover
func (c *Cluster) ReportNodeDown(nodeID string) {
	c.mu.Lock()
	if node, exists := c.nodes[nodeID]; exists {
		node.Healthy = false
		c.log.Warn("Cluster: node %s marked as down", nodeID)
	}
	c.mu.Unlock()
	c.failovers.Add(1)
}

// ReportNodeUp marks a node as healthy
func (c *Cluster) ReportNodeUp(nodeID string) {
	c.mu.Lock()
	if node, exists := c.nodes[nodeID]; exists {
		node.Healthy = true
		node.LastSeen = time.Now()
	}
	c.mu.Unlock()
}

// UpdateLocalLoad updates this node's load information
func (c *Cluster) UpdateLocalLoad(activeConns int, load float64) {
	c.mu.Lock()
	c.localNode.ActiveConns = activeConns
	c.localNode.Load = load
	c.localNode.LastSeen = time.Now()
	c.mu.Unlock()
}

// GetNodes returns all nodes
func (c *Cluster) GetNodes() []*Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]*Node, 0, len(c.nodes))
	for _, n := range c.nodes {
		nodeCopy := *n
		nodes = append(nodes, &nodeCopy)
	}
	return nodes
}

// healthCheckLoop periodically checks peer health
func (c *Cluster) healthCheckLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.checkPeers()
		}
	}
}

// checkPeers checks health of all peer nodes
func (c *Cluster) checkPeers() {
	c.mu.RLock()
	nodes := make([]*Node, 0)
	for _, n := range c.nodes {
		if n.ID != c.localNode.ID {
			nodes = append(nodes, n)
		}
	}
	c.mu.RUnlock()

	for _, node := range nodes {
		go func(n *Node) {
			start := time.Now()
			healthy := c.pingNode(n.Address)
			latency := time.Since(start).Milliseconds()

			c.mu.Lock()
			n.Healthy = healthy
			if healthy {
				n.Latency = latency
				n.LastSeen = time.Now()
			}
			c.mu.Unlock()
		}(node)
	}
}

// pingNode checks if a node is reachable
func (c *Cluster) pingNode(addr string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/cluster/health", addr))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// syncLoop synchronizes state with peers
func (c *Cluster) syncLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.syncState()
		}
	}
}

// syncState pushes local state to peers
func (c *Cluster) syncState() {
	c.mu.RLock()
	localState, _ := json.Marshal(c.localNode)
	peers := make([]string, 0)
	for _, n := range c.nodes {
		if n.ID != c.localNode.ID && n.Healthy {
			peers = append(peers, n.Address)
		}
	}
	c.mu.RUnlock()

	client := &http.Client{Timeout: 5 * time.Second}
	for _, addr := range peers {
		go func(a string) {
			resp, err := client.Post(
				fmt.Sprintf("http://%s/cluster/sync", a),
				"application/json",
				strings.NewReader(string(localState)),
			)
			if err == nil {
				resp.Body.Close()
			}
		}(addr)
	}
}

// startAPI starts the cluster management API
func (c *Cluster) startAPI() {
	mux := http.NewServeMux()
	mux.HandleFunc("/cluster/health", c.handleHealth)
	mux.HandleFunc("/cluster/nodes", c.handleNodes)
	mux.HandleFunc("/cluster/sync", c.handleSync)
	mux.HandleFunc("/cluster/stats", c.handleStats)

	server := &http.Server{
		Addr:    c.apiAddr,
		Handler: mux,
	}

	c.log.Info("Cluster API on %s", c.apiAddr)
	server.ListenAndServe()
}

func (c *Cluster) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"node_id": c.localNode.ID,
		"load":    c.localNode.Load,
	})
}

func (c *Cluster) handleNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.GetNodes())
}

func (c *Cluster) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

	var node Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		http.Error(w, "invalid body", 400)
		return
	}

	c.mu.Lock()
	if existing, ok := c.nodes[node.ID]; ok {
		existing.Load = node.Load
		existing.ActiveConns = node.ActiveConns
		existing.LastSeen = time.Now()
		existing.Healthy = true
	} else {
		node.Healthy = true
		node.LastSeen = time.Now()
		c.nodes[node.ID] = &node
	}
	c.mu.Unlock()

	w.WriteHeader(200)
}

func (c *Cluster) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_requests": c.totalRequests.Load(),
		"failovers":      c.failovers.Load(),
		"nodes":          len(c.nodes),
	})
}

// Stop shuts down the cluster
func (c *Cluster) Stop() {
	close(c.done)
	c.wg.Wait()
}
