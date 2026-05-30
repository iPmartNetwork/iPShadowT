package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Manager handles all security features
type Manager struct {
	log          *logger.Logger
	ipWhitelist  map[string]bool
	ipBlacklist  map[string]bool
	auditLog     []AuditEntry
	auditMu      sync.Mutex
	certPins     []string // SHA-256 hashes of pinned certificates
	failedLogins sync.Map // IP → failure count
	mu           sync.RWMutex
}

// AuditEntry represents a security audit log entry
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Event     string    `json:"event"`
	Source    string    `json:"source"`
	UserID    string    `json:"user_id"`
	Details   string    `json:"details"`
	Success   bool      `json:"success"`
}

// NewManager creates a new security manager
func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		log:         log,
		ipWhitelist: make(map[string]bool),
		ipBlacklist: make(map[string]bool),
		auditLog:    make([]AuditEntry, 0, 1000),
	}
}

// --- IP Whitelist/Blacklist ---

// AddToWhitelist adds an IP to the whitelist
func (m *Manager) AddToWhitelist(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ipWhitelist[ip] = true
	m.log.Info("Security: IP whitelisted: %s", ip)
}

// AddToBlacklist adds an IP to the blacklist
func (m *Manager) AddToBlacklist(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ipBlacklist[ip] = true
	m.log.Info("Security: IP blacklisted: %s", ip)
}

// IsAllowed checks if an IP is allowed to connect
func (m *Manager) IsAllowed(addr string) bool {
	host, _, _ := net.SplitHostPort(addr)
	if host == "" {
		host = addr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// If blacklist has entries, check it first
	if len(m.ipBlacklist) > 0 && m.ipBlacklist[host] {
		return false
	}

	// If whitelist has entries, only allow listed IPs
	if len(m.ipWhitelist) > 0 {
		return m.ipWhitelist[host]
	}

	return true // No restrictions
}

// --- Certificate Pinning ---

// AddCertPin adds a certificate pin (SHA-256 hash)
func (m *Manager) AddCertPin(sha256Hash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.certPins = append(m.certPins, sha256Hash)
}

// VerifyCertPin checks if a certificate matches a pinned hash
func (m *Manager) VerifyCertPin(certDER []byte) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.certPins) == 0 {
		return true // No pins configured
	}

	hash := sha256.Sum256(certDER)
	certHash := hex.EncodeToString(hash[:])

	for _, pin := range m.certPins {
		if subtle.ConstantTimeCompare([]byte(certHash), []byte(pin)) == 1 {
			return true
		}
	}

	return false
}

// --- Brute Force Protection ---

// RecordFailedLogin records a failed login attempt
func (m *Manager) RecordFailedLogin(ip string) bool {
	val, _ := m.failedLogins.LoadOrStore(ip, &loginAttempts{})
	attempts := val.(*loginAttempts)
	attempts.mu.Lock()
	defer attempts.mu.Unlock()

	attempts.count++
	attempts.lastAttempt = time.Now()

	// Block after 5 failures within 5 minutes
	if attempts.count >= 5 {
		m.AddToBlacklist(ip)
		m.Audit("brute_force_blocked", ip, "", fmt.Sprintf("Blocked after %d failed attempts", attempts.count), false)
		return true // Blocked
	}

	return false
}

// ResetFailedLogins resets failed login counter for an IP
func (m *Manager) ResetFailedLogins(ip string) {
	m.failedLogins.Delete(ip)
}

type loginAttempts struct {
	count       int
	lastAttempt time.Time
	mu          sync.Mutex
}

// --- Audit Log ---

// Audit records a security event
func (m *Manager) Audit(event, source, userID, details string, success bool) {
	entry := AuditEntry{
		Timestamp: time.Now(),
		Event:     event,
		Source:    source,
		UserID:    userID,
		Details:   details,
		Success:   success,
	}

	m.auditMu.Lock()
	m.auditLog = append(m.auditLog, entry)
	// Keep last 10000 entries
	if len(m.auditLog) > 10000 {
		m.auditLog = m.auditLog[len(m.auditLog)-10000:]
	}
	m.auditMu.Unlock()

	if !success {
		m.log.Warn("AUDIT [%s] %s from %s: %s", event, userID, source, details)
	} else {
		m.log.Debug("AUDIT [%s] %s from %s: %s", event, userID, source, details)
	}
}

// GetAuditLog returns recent audit entries
func (m *Manager) GetAuditLog(limit int) []AuditEntry {
	m.auditMu.Lock()
	defer m.auditMu.Unlock()

	if limit <= 0 || limit > len(m.auditLog) {
		limit = len(m.auditLog)
	}

	start := len(m.auditLog) - limit
	result := make([]AuditEntry, limit)
	copy(result, m.auditLog[start:])
	return result
}

// --- Cleanup ---

// CleanupExpiredBlocks removes expired IP blocks
func (m *Manager) CleanupExpiredBlocks() {
	m.failedLogins.Range(func(key, value interface{}) bool {
		attempts := value.(*loginAttempts)
		attempts.mu.Lock()
		if time.Since(attempts.lastAttempt) > 30*time.Minute {
			m.failedLogins.Delete(key)
			// Also remove from blacklist if it was auto-added
			m.mu.Lock()
			delete(m.ipBlacklist, key.(string))
			m.mu.Unlock()
		}
		attempts.mu.Unlock()
		return true
	})
}
