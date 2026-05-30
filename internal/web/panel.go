package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/iPmart/iPShadowT/internal/health"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/users"
)

// Panel provides a simple web management panel
type Panel struct {
	addr        string
	log         *logger.Logger
	userMgr     *users.Manager
	healthSvc   *health.Service
	server      *http.Server
	adminUser   string
	adminPass   string
}

// PanelConfig holds panel configuration
type PanelConfig struct {
	Addr      string // Listen address (e.g., "127.0.0.1:8080")
	AdminUser string // Admin username
	AdminPass string // Admin password
}

// NewPanel creates a new web panel
func NewPanel(cfg PanelConfig, userMgr *users.Manager, healthSvc *health.Service, log *logger.Logger) *Panel {
	return &Panel{
		addr:      cfg.Addr,
		log:       log,
		userMgr:   userMgr,
		healthSvc: healthSvc,
		adminUser: cfg.AdminUser,
		adminPass: cfg.AdminPass,
	}
}

// Start begins serving the web panel
func (p *Panel) Start() error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/status", p.authMiddleware(p.handleStatus))
	mux.HandleFunc("/api/users", p.authMiddleware(p.handleUsers))
	mux.HandleFunc("/api/users/add", p.authMiddleware(p.handleAddUser))
	mux.HandleFunc("/api/users/remove", p.authMiddleware(p.handleRemoveUser))
	mux.HandleFunc("/api/stats", p.authMiddleware(p.handleStats))

	// Web UI
	mux.HandleFunc("/", p.authMiddleware(p.handleDashboard))

	p.server = &http.Server{
		Addr:         p.addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.log.Error("Web panel error: %v", err)
		}
	}()

	p.log.Info("Web panel listening on %s", p.addr)
	return nil
}

// authMiddleware provides basic authentication
func (p *Panel) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p.adminUser != "" && p.adminPass != "" {
			user, pass, ok := r.BasicAuth()
			if !ok || user != p.adminUser || pass != p.adminPass {
				w.Header().Set("WWW-Authenticate", `Basic realm="iPShadowT"`)
				http.Error(w, "Unauthorized", 401)
				return
			}
		}
		next(w, r)
	}
}

// handleStatus returns server status
func (p *Panel) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":  "running",
		"version": "v1.0.0",
		"time":    time.Now().Format(time.RFC3339),
	}
	jsonResponse(w, status)
}

// handleUsers returns user list
func (p *Panel) handleUsers(w http.ResponseWriter, r *http.Request) {
	if p.userMgr == nil {
		jsonResponse(w, map[string]string{"error": "user management not enabled"})
		return
	}
	jsonResponse(w, p.userMgr.GetStats())
}

// handleAddUser adds a new user
func (p *Panel) handleAddUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		MaxTraffic int64  `json:"max_traffic"`
		ExpiresAt  string `json:"expires_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}

	if req.ID == "" || req.Name == "" {
		jsonError(w, "id and name are required", 400)
		return
	}

	// Generate short ID for the user
	shortID, _ := GenerateShortID()

	user := &users.User{
		ID:         req.ID,
		Name:       req.Name,
		ShortID:    shortID,
		MaxTraffic: req.MaxTraffic,
		ExpiresAt:  req.ExpiresAt,
		Enabled:    true,
	}

	if err := p.userMgr.AddUser(user); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success":  true,
		"user":     user,
		"short_id": shortID,
	})
}

// handleRemoveUser removes a user
func (p *Panel) handleRemoveUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}

	if err := p.userMgr.RemoveUser(req.ID); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	jsonResponse(w, map[string]string{"success": "true"})
}

// handleStats returns detailed statistics
func (p *Panel) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"users": p.userMgr.GetStats(),
		"time":  time.Now().Format(time.RFC3339),
	}
	jsonResponse(w, stats)
}

// handleDashboard serves the web UI
func (p *Panel) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

// Stop shuts down the panel
func (p *Panel) Stop() error {
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// GenerateShortID generates a random short ID
func GenerateShortID() (string, error) {
	return fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFFFF), nil
}

// Dashboard HTML
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>iPShadowT Panel</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #0f172a; color: #e2e8f0; padding: 20px; }
.container { max-width: 1200px; margin: 0 auto; }
h1 { color: #38bdf8; margin-bottom: 20px; font-size: 24px; }
.card { background: #1e293b; border-radius: 12px; padding: 20px; margin-bottom: 16px; border: 1px solid #334155; }
.card h2 { color: #94a3b8; font-size: 14px; text-transform: uppercase; margin-bottom: 12px; }
.stat { display: inline-block; margin-right: 30px; }
.stat .value { font-size: 28px; font-weight: bold; color: #38bdf8; }
.stat .label { font-size: 12px; color: #64748b; }
table { width: 100%; border-collapse: collapse; margin-top: 10px; }
th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid #334155; }
th { color: #94a3b8; font-size: 12px; text-transform: uppercase; }
.badge { padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.badge-green { background: #064e3b; color: #34d399; }
.badge-red { background: #450a0a; color: #f87171; }
.btn { padding: 8px 16px; border: none; border-radius: 6px; cursor: pointer; font-size: 14px; }
.btn-primary { background: #2563eb; color: white; }
.btn-danger { background: #dc2626; color: white; }
#status { color: #34d399; }
</style>
</head>
<body>
<div class="container">
<h1>🛡️ iPShadowT Panel</h1>
<div class="card">
<h2>Server Status</h2>
<div class="stat"><div class="value" id="status">Loading...</div><div class="label">Status</div></div>
<div class="stat"><div class="value" id="uptime">-</div><div class="label">Uptime</div></div>
<div class="stat"><div class="value" id="connections">0</div><div class="label">Connections</div></div>
</div>
<div class="card">
<h2>Users</h2>
<table>
<thead><tr><th>Name</th><th>Upload</th><th>Download</th><th>Status</th><th>Last Seen</th></tr></thead>
<tbody id="users-table"><tr><td colspan="5">Loading...</td></tr></tbody>
</table>
</div>
</div>
<script>
async function refresh() {
  try {
    const r = await fetch('/api/stats');
    const data = await r.json();
    document.getElementById('status').textContent = 'Online';
    if (data.users) {
      let html = '';
      data.users.forEach(u => {
        const up = formatBytes(u.bytes_up);
        const down = formatBytes(u.bytes_down);
        const badge = u.enabled ? '<span class="badge badge-green">Active</span>' : '<span class="badge badge-red">Disabled</span>';
        html += '<tr><td>'+u.name+'</td><td>'+up+'</td><td>'+down+'</td><td>'+badge+'</td><td>'+(u.last_seen||'Never')+'</td></tr>';
      });
      document.getElementById('users-table').innerHTML = html || '<tr><td colspan="5">No users</td></tr>';
    }
  } catch(e) { document.getElementById('status').textContent = 'Error'; }
}
function formatBytes(b) {
  if (b > 1073741824) return (b/1073741824).toFixed(2)+' GB';
  if (b > 1048576) return (b/1048576).toFixed(2)+' MB';
  if (b > 1024) return (b/1024).toFixed(2)+' KB';
  return b+' B';
}
refresh();
setInterval(refresh, 5000);
</script>
</body>
</html>`
