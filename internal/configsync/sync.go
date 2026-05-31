package configsync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Syncer synchronizes configuration between server and client nodes
type Syncer struct {
	log        *logger.Logger
	localPath  string
	remotePath string
	password   string
	role       string // "push" or "pull"
	interval   time.Duration
	peerAddr   string
	listenAddr string
	done       chan struct{}
	wg         sync.WaitGroup
	version    int64
	mu         sync.RWMutex
	onChange   func(newConfig []byte)
}

// SyncConfig configures the config syncer
type SyncConfig struct {
	LocalPath  string        // Path to local config file
	Password   string        // Encryption password for sync
	Role       string        // "push" (server) or "pull" (client)
	PeerAddr   string        // Address of the peer to sync with
	ListenAddr string        // Address to listen for sync requests (push mode)
	Interval   time.Duration // Sync interval
	OnChange   func(newConfig []byte) // Callback when config changes
}

// NewSyncer creates a new config syncer
func NewSyncer(cfg SyncConfig, log *logger.Logger) *Syncer {
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}

	return &Syncer{
		log:        log,
		localPath:  cfg.LocalPath,
		password:   cfg.Password,
		role:       cfg.Role,
		peerAddr:   cfg.PeerAddr,
		listenAddr: cfg.ListenAddr,
		interval:   cfg.Interval,
		done:       make(chan struct{}),
		onChange:   cfg.OnChange,
	}
}

// Start begins config synchronization
func (s *Syncer) Start() error {
	// Calculate initial version
	s.updateVersion()

	if s.role == "push" && s.listenAddr != "" {
		// Server mode: serve config to clients
		go s.startServer()
	}

	if s.role == "pull" && s.peerAddr != "" {
		// Client mode: periodically pull config from server
		s.wg.Add(1)
		go s.pullLoop()
	}

	s.log.Info("ConfigSync: started (role=%s, interval=%v)", s.role, s.interval)
	return nil
}

// startServer starts the config sync HTTP server
func (s *Syncer) startServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/config/version", s.handleVersion)
	mux.HandleFunc("/config/pull", s.handlePull)
	mux.HandleFunc("/config/push", s.handlePush)

	server := &http.Server{
		Addr:    s.listenAddr,
		Handler: mux,
	}

	s.log.Info("ConfigSync: serving on %s", s.listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.log.Error("ConfigSync server error: %v", err)
	}
}

// handleVersion returns the current config version
func (s *Syncer) handleVersion(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ver := s.version
	s.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]int64{"version": ver})
}

// handlePull returns the encrypted config
func (s *Syncer) handlePull(w http.ResponseWriter, r *http.Request) {
	// Read local config
	data, err := os.ReadFile(s.localPath)
	if err != nil {
		http.Error(w, "config not found", 404)
		return
	}

	// Encrypt
	encrypted, err := s.encrypt(data)
	if err != nil {
		http.Error(w, "encryption failed", 500)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Config-Version", fmt.Sprintf("%d", s.version))
	w.Write(encrypted)
}

// handlePush receives a config push from a peer
func (s *Syncer) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read failed", 400)
		return
	}

	// Decrypt
	decrypted, err := s.decrypt(body)
	if err != nil {
		http.Error(w, "decryption failed", 403)
		return
	}

	// Write to local config
	if err := os.WriteFile(s.localPath, decrypted, 0600); err != nil {
		http.Error(w, "write failed", 500)
		return
	}

	s.updateVersion()

	if s.onChange != nil {
		s.onChange(decrypted)
	}

	s.log.Info("ConfigSync: received config push (version: %d)", s.version)
	w.WriteHeader(200)
}

// pullLoop periodically pulls config from the server
func (s *Syncer) pullLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.pullConfig()
		}
	}
}

// pullConfig pulls config from the peer
func (s *Syncer) pullConfig() {
	client := &http.Client{Timeout: 10 * time.Second}

	// Check version first
	resp, err := client.Get(fmt.Sprintf("http://%s/config/version", s.peerAddr))
	if err != nil {
		s.log.Debug("ConfigSync: version check failed: %v", err)
		return
	}
	defer resp.Body.Close()

	var versionResp struct {
		Version int64 `json:"version"`
	}
	json.NewDecoder(resp.Body).Decode(&versionResp)

	s.mu.RLock()
	localVersion := s.version
	s.mu.RUnlock()

	if versionResp.Version <= localVersion {
		return // Already up to date
	}

	// Pull new config
	resp2, err := client.Get(fmt.Sprintf("http://%s/config/pull", s.peerAddr))
	if err != nil {
		s.log.Error("ConfigSync: pull failed: %v", err)
		return
	}
	defer resp2.Body.Close()

	body, err := io.ReadAll(resp2.Body)
	if err != nil {
		return
	}

	// Decrypt
	decrypted, err := s.decrypt(body)
	if err != nil {
		s.log.Error("ConfigSync: decryption failed: %v", err)
		return
	}

	// Write to local config
	if err := os.WriteFile(s.localPath, decrypted, 0600); err != nil {
		s.log.Error("ConfigSync: write failed: %v", err)
		return
	}

	s.updateVersion()
	s.log.Info("ConfigSync: pulled new config (version: %d)", s.version)

	if s.onChange != nil {
		s.onChange(decrypted)
	}
}

// PushToClient pushes config to a specific client
func (s *Syncer) PushToClient(clientAddr string) error {
	data, err := os.ReadFile(s.localPath)
	if err != nil {
		return fmt.Errorf("read config failed: %w", err)
	}

	encrypted, err := s.encrypt(data)
	if err != nil {
		return fmt.Errorf("encrypt failed: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("http://%s/config/push", clientAddr),
		"application/octet-stream",
		io.NopCloser(io.Reader(nil)),
	)
	_ = encrypted
	if err != nil {
		return fmt.Errorf("push failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("push rejected: status %d", resp.StatusCode)
	}

	return nil
}

// updateVersion calculates config version from file modification time
func (s *Syncer) updateVersion() {
	info, err := os.Stat(s.localPath)
	if err == nil {
		s.mu.Lock()
		s.version = info.ModTime().UnixMilli()
		s.mu.Unlock()
	}
}

// encrypt encrypts data using AES-GCM with the shared password
func (s *Syncer) encrypt(plaintext []byte) ([]byte, error) {
	key := sha256.Sum256([]byte(s.password))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt decrypts data using AES-GCM with the shared password
func (s *Syncer) decrypt(ciphertext []byte) ([]byte, error) {
	key := sha256.Sum256([]byte(s.password))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// Stop shuts down the syncer
func (s *Syncer) Stop() {
	close(s.done)
	s.wg.Wait()
}
