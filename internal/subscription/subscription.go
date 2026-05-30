package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/users"
)

// Service provides subscription links for clients
// Compatible with common proxy clients (V2RayNG, Clash, etc.)
type Service struct {
	addr      string
	serverIP  string
	serverPort string
	transport string
	log       *logger.Logger
	userMgr   *users.Manager
	server    *http.Server
}

// Config holds subscription service configuration
type Config struct {
	Addr       string // Listen address
	ServerIP   string // Public server IP/domain
	ServerPort string // Server port
	Transport  string // Transport type
}

// NewService creates a new subscription service
func NewService(cfg Config, userMgr *users.Manager, log *logger.Logger) *Service {
	return &Service{
		addr:       cfg.Addr,
		serverIP:   cfg.ServerIP,
		serverPort: cfg.ServerPort,
		transport:  cfg.Transport,
		log:        log,
		userMgr:    userMgr,
	}
}

// Start begins serving subscription links
func (s *Service) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/sub/", s.handleSubscription)
	mux.HandleFunc("/clash/", s.handleClash)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("Subscription service error: %v", err)
		}
	}()

	s.log.Info("Subscription service on %s", s.addr)
	return nil
}

// handleSubscription returns base64-encoded proxy config for a user
// URL format: /sub/{user_id}
func (s *Service) handleSubscription(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimPrefix(r.URL.Path, "/sub/")
	if userID == "" {
		http.Error(w, "User ID required", 400)
		return
	}

	user, exists := s.userMgr.GetUser(userID)
	if !exists {
		http.Error(w, "Not found", 404)
		return
	}

	if !user.Enabled {
		http.Error(w, "User disabled", 403)
		return
	}

	// Generate proxy links
	links := s.generateLinks(user)

	// Base64 encode (standard subscription format)
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=ipshadowt.txt")
	w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=0; total=%d", user.MaxTraffic))
	w.Write([]byte(encoded))
}

// handleClash returns Clash-compatible YAML config
func (s *Service) handleClash(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimPrefix(r.URL.Path, "/clash/")
	if userID == "" {
		http.Error(w, "User ID required", 400)
		return
	}

	user, exists := s.userMgr.GetUser(userID)
	if !exists {
		http.Error(w, "Not found", 404)
		return
	}

	clash := s.generateClashConfig(user)

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Write([]byte(clash))
}

// generateLinks generates proxy links for a user
func (s *Service) generateLinks(user *users.User) []string {
	links := make([]string, 0)

	// iPShadowT native link
	nativeLink := fmt.Sprintf("ipshadowt://%s@%s:%s?transport=%s&shortid=%s#%s",
		user.ID, s.serverIP, s.serverPort, s.transport, user.ShortID, user.Name)
	links = append(links, nativeLink)

	// SOCKS5 link (for generic clients)
	socks5Link := fmt.Sprintf("socks5://127.0.0.1:1080#%s-SOCKS5", user.Name)
	links = append(links, socks5Link)

	return links
}

// generateClashConfig generates Clash-compatible YAML
func (s *Service) generateClashConfig(user *users.User) string {
	config := fmt.Sprintf(`# iPShadowT - Clash Config for %s
# Auto-generated

proxies:
  - name: "%s"
    type: socks5
    server: 127.0.0.1
    port: 1080

proxy-groups:
  - name: "iPShadowT"
    type: select
    proxies:
      - "%s"
      - DIRECT

rules:
  - GEOIP,IR,DIRECT
  - MATCH,iPShadowT
`, user.Name, user.Name, user.Name)

	return config
}

// GenerateSubLink returns the subscription URL for a user
func GenerateSubLink(baseURL, userID string) string {
	return fmt.Sprintf("%s/sub/%s", baseURL, userID)
}

// SubInfo holds subscription info in JSON format
type SubInfo struct {
	ServerIP   string `json:"server_ip"`
	ServerPort string `json:"server_port"`
	Transport  string `json:"transport"`
	ShortID    string `json:"short_id"`
	UserID     string `json:"user_id"`
	SubURL     string `json:"sub_url"`
	ClashURL   string `json:"clash_url"`
}

// GetSubInfo returns subscription info for a user
func (s *Service) GetSubInfo(userID string) (*SubInfo, error) {
	user, exists := s.userMgr.GetUser(userID)
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	baseURL := fmt.Sprintf("http://%s", s.addr)

	return &SubInfo{
		ServerIP:   s.serverIP,
		ServerPort: s.serverPort,
		Transport:  s.transport,
		ShortID:    user.ShortID,
		UserID:     user.ID,
		SubURL:     fmt.Sprintf("%s/sub/%s", baseURL, userID),
		ClashURL:   fmt.Sprintf("%s/clash/%s", baseURL, userID),
	}, nil
}

// Stop shuts down the subscription service
func (s *Service) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// SerializeForQR returns a JSON string suitable for QR code generation
func SerializeForQR(info *SubInfo) string {
	data, _ := json.Marshal(info)
	return string(data)
}
