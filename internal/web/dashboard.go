package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iPmart/iPShadowT/internal/health"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/metrics"
	"github.com/iPmart/iPShadowT/internal/users"
)

// Dashboard provides a real-time web management dashboard
type Dashboard struct {
	addr      string
	log       *logger.Logger
	userMgr   *users.Manager
	healthSvc *health.Service
	metrics   *metrics.Collector
	server    *http.Server
	adminUser string
	adminPass string
	wsClients sync.Map // WebSocket clients for real-time updates
	upgrader  websocket.Upgrader
}

// DashboardConfig holds dashboard configuration
type DashboardConfig struct {
	Addr      string
	AdminUser string
	AdminPass string
}

// NewDashboard creates a new real-time dashboard
func NewDashboard(cfg DashboardConfig, userMgr *users.Manager, healthSvc *health.Service, met *metrics.Collector, log *logger.Logger) *Dashboard {
	return &Dashboard{
		addr:      cfg.Addr,
		log:       log,
		userMgr:   userMgr,
		healthSvc: healthSvc,
		metrics:   met,
		adminUser: cfg.AdminUser,
		adminPass: cfg.AdminPass,
		upgrader: websocket.Upgrader{
			CheckOrigin:     func(r *http.Request) bool { return true },
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// Start begins serving the dashboard
func (d *Dashboard) Start() error {
	mux := http.NewServeMux()

	// WebSocket endpoint for real-time data
	mux.HandleFunc("/ws", d.authMiddleware(d.handleWebSocket))

	// REST API endpoints
	mux.HandleFunc("/api/dashboard/stats", d.authMiddleware(d.handleDashboardStats))
	mux.HandleFunc("/api/dashboard/tunnels", d.authMiddleware(d.handleTunnels))
	mux.HandleFunc("/api/dashboard/traffic", d.authMiddleware(d.handleTrafficHistory))
	mux.HandleFunc("/api/dashboard/failover", d.authMiddleware(d.handleFailoverStatus))
	mux.HandleFunc("/api/dashboard/users", d.authMiddleware(d.handleUserManagement))
	mux.HandleFunc("/api/dashboard/config", d.authMiddleware(d.handleConfigView))

	// Serve the dashboard UI
	mux.HandleFunc("/", d.authMiddleware(d.handleDashboardUI))

	d.server = &http.Server{
		Addr:         d.addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			d.log.Error("Dashboard error: %v", err)
		}
	}()

	// Start broadcasting stats to WebSocket clients
	go d.broadcastLoop()

	d.log.Info("Dashboard listening on %s", d.addr)
	return nil
}

// authMiddleware provides basic authentication
func (d *Dashboard) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.adminUser != "" && d.adminPass != "" {
			user, pass, ok := r.BasicAuth()
			if !ok || user != d.adminUser || pass != d.adminPass {
				w.Header().Set("WWW-Authenticate", `Basic realm="iPShadowT Dashboard"`)
				http.Error(w, "Unauthorized", 401)
				return
			}
		}
		next(w, r)
	}
}

// handleWebSocket upgrades to WebSocket for real-time updates
func (d *Dashboard) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := d.upgrader.Upgrade(w, r, nil)
	if err != nil {
		d.log.Error("WebSocket upgrade failed: %v", err)
		return
	}

	clientID := fmt.Sprintf("%p", conn)
	d.wsClients.Store(clientID, conn)

	defer func() {
		d.wsClients.Delete(clientID)
		conn.Close()
	}()

	// Keep connection alive and handle incoming messages
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// broadcastLoop sends real-time stats to all WebSocket clients
func (d *Dashboard) broadcastLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := d.collectRealtimeStats()
		data, _ := json.Marshal(stats)

		d.wsClients.Range(func(key, value interface{}) bool {
			conn := value.(*websocket.Conn)
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				d.wsClients.Delete(key)
				conn.Close()
			}
			return true
		})
	}
}

// RealtimeStats holds real-time dashboard data
type RealtimeStats struct {
	Timestamp         int64   `json:"timestamp"`
	ActiveConnections int64   `json:"active_connections"`
	TotalConnections  int64   `json:"total_connections"`
	BytesIn           int64   `json:"bytes_in"`
	BytesOut          int64   `json:"bytes_out"`
	BytesInRate       float64 `json:"bytes_in_rate"`
	BytesOutRate      float64 `json:"bytes_out_rate"`
	ActiveStreams     int64   `json:"active_streams"`
	UptimeSeconds     float64 `json:"uptime_seconds"`
	ActivePath        string  `json:"active_path"`
	PathCount         int     `json:"path_count"`
	UsersOnline       int     `json:"users_online"`
}

var lastBytesIn, lastBytesOut int64
var lastStatsTime time.Time

func (d *Dashboard) collectRealtimeStats() RealtimeStats {
	now := time.Now()
	stats := RealtimeStats{
		Timestamp: now.UnixMilli(),
	}

	if d.metrics != nil {
		snapshot := d.metrics.GetSnapshot()
		stats.ActiveConnections = snapshot.ActiveConnections
		stats.TotalConnections = snapshot.TotalConnections
		stats.BytesIn = snapshot.BytesReceived
		stats.BytesOut = snapshot.BytesSent
		stats.ActiveStreams = snapshot.ActiveStreams
		stats.UptimeSeconds = snapshot.Uptime

		// Calculate rates
		elapsed := now.Sub(lastStatsTime).Seconds()
		if elapsed > 0 && lastStatsTime.Unix() > 0 {
			stats.BytesInRate = float64(stats.BytesIn-lastBytesIn) / elapsed
			stats.BytesOutRate = float64(stats.BytesOut-lastBytesOut) / elapsed
		}
		lastBytesIn = stats.BytesIn
		lastBytesOut = stats.BytesOut
		lastStatsTime = now
	}

	if d.userMgr != nil {
		userStats := d.userMgr.GetStats()
		for _, u := range userStats {
			if u.LastSeen != "" {
				stats.UsersOnline++
			}
		}
	}

	return stats
}

func (d *Dashboard) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	stats := d.collectRealtimeStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (d *Dashboard) handleTunnels(w http.ResponseWriter, r *http.Request) {
	// Return tunnel status info
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (d *Dashboard) handleTrafficHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (d *Dashboard) handleFailoverStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (d *Dashboard) handleUserManagement(w http.ResponseWriter, r *http.Request) {
	if d.userMgr == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "user management not enabled"})
		return
	}

	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(d.userMgr.GetStats())
	case "POST":
		var req struct {
			Action string `json:"action"`
			ID     string `json:"id"`
			Name   string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Action {
		case "add":
			user := &users.User{ID: req.ID, Name: req.Name, Enabled: true}
			d.userMgr.AddUser(user)
		case "remove":
			d.userMgr.RemoveUser(req.ID)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func (d *Dashboard) handleConfigView(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleDashboardUI serves the full dashboard HTML
func (d *Dashboard) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(fullDashboardHTML))
}

// Stop shuts down the dashboard
func (d *Dashboard) Stop() error {
	if d.server != nil {
		return d.server.Close()
	}
	return nil
}
