package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/iPmart/iPShadowT/internal/health"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/metrics"
	"github.com/iPmart/iPShadowT/internal/security"
	"github.com/iPmart/iPShadowT/internal/users"
)

// Server provides a REST API for external management
// Can be used by mobile apps, web dashboards, or other services
type Server struct {
	addr      string
	log       *logger.Logger
	userMgr   *users.Manager
	healthSvc *health.Service
	metrics   *metrics.Collector
	security  *security.Manager
	apiKey    string
	server    *http.Server
}

// Config holds API server configuration
type Config struct {
	Addr   string // Listen address
	APIKey string // API key for authentication
}

// NewServer creates a new API server
func NewServer(cfg Config, userMgr *users.Manager, healthSvc *health.Service, met *metrics.Collector, sec *security.Manager, log *logger.Logger) *Server {
	return &Server{
		addr:      cfg.Addr,
		log:       log,
		userMgr:   userMgr,
		healthSvc: healthSvc,
		metrics:   met,
		security:  sec,
		apiKey:    cfg.APIKey,
	}
}

// Start begins serving the API
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// System endpoints
	mux.HandleFunc("/api/v1/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/v1/metrics", s.auth(s.handleMetrics))
	mux.HandleFunc("/api/v1/health", s.handleHealth) // No auth needed

	// User management
	mux.HandleFunc("/api/v1/users", s.auth(s.handleListUsers))
	mux.HandleFunc("/api/v1/users/create", s.auth(s.handleCreateUser))
	mux.HandleFunc("/api/v1/users/delete", s.auth(s.handleDeleteUser))
	mux.HandleFunc("/api/v1/users/toggle", s.auth(s.handleToggleUser))

	// Security
	mux.HandleFunc("/api/v1/security/audit", s.auth(s.handleAuditLog))
	mux.HandleFunc("/api/v1/security/blacklist", s.auth(s.handleBlacklist))

	// Control
	mux.HandleFunc("/api/v1/control/restart", s.auth(s.handleRestart))

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("API server error: %v", err)
		}
	}()

	s.log.Info("REST API listening on %s", s.addr)
	return nil
}

// auth middleware checks API key
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				key = r.URL.Query().Get("api_key")
			}
			if key != s.apiKey {
				jsonErr(w, "unauthorized", 401)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":  "running",
		"version": "v1.0.0",
		"author":  "iPmart Network (Ali Hassanzadeh)",
		"uptime":  time.Since(time.Now()).String(), // placeholder
	}
	jsonOK(w, status)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.metrics != nil {
		jsonOK(w, s.metrics.GetSnapshot())
	} else {
		jsonOK(w, map[string]string{"status": "metrics not enabled"})
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "healthy"})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if s.userMgr == nil {
		jsonErr(w, "user management not enabled", 400)
		return
	}
	jsonOK(w, s.userMgr.GetStats())
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonErr(w, "method not allowed", 405)
		return
	}

	var req struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		MaxTraffic int64  `json:"max_traffic"`
		ExpiresAt  string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body", 400)
		return
	}

	user := &users.User{
		ID:         req.ID,
		Name:       req.Name,
		MaxTraffic: req.MaxTraffic,
		ExpiresAt:  req.ExpiresAt,
		Enabled:    true,
	}

	if err := s.userMgr.AddUser(user); err != nil {
		jsonErr(w, err.Error(), 400)
		return
	}

	s.security.Audit("user_created", r.RemoteAddr, req.ID, "User created via API", true)
	jsonOK(w, map[string]string{"status": "created", "id": req.ID})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonErr(w, "method not allowed", 405)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if err := s.userMgr.RemoveUser(req.ID); err != nil {
		jsonErr(w, err.Error(), 400)
		return
	}

	s.security.Audit("user_deleted", r.RemoteAddr, req.ID, "User deleted via API", true)
	jsonOK(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleToggleUser(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented yet"})
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if s.security != nil {
		jsonOK(w, s.security.GetAuditLog(100))
	}
}

func (s *Server) handleBlacklist(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var req struct {
			IP string `json:"ip"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		s.security.AddToBlacklist(req.IP)
		jsonOK(w, map[string]string{"status": "blocked", "ip": req.IP})
		return
	}
	jsonOK(w, map[string]string{"status": "use POST to add"})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "restart signal sent"})
	// In production, this would signal the main process to restart
}

// Stop shuts down the API server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateAPIKey generates a random API key
func GenerateAPIKey() string {
	return fmt.Sprintf("ipst_%d", time.Now().UnixNano())
}
