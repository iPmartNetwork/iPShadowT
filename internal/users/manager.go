package users

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// User represents a tunnel user
type User struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Password    string    `json:"password"`     // Per-user password (optional)
	ShortID     string    `json:"short_id"`     // REALITY short ID
	MaxTraffic  int64     `json:"max_traffic"`  // Max traffic in bytes (0 = unlimited)
	ExpiresAt   string    `json:"expires_at"`   // Expiry date (RFC3339, empty = never)
	Enabled     bool      `json:"enabled"`
	CreatedAt   string    `json:"created_at"`

	// Runtime stats (not persisted)
	BytesUp     atomic.Int64 `json:"-"`
	BytesDown   atomic.Int64 `json:"-"`
	Connections atomic.Int64 `json:"-"`
	LastSeen    time.Time    `json:"-"`
}

// Manager handles user management
type Manager struct {
	users    map[string]*User
	mu       sync.RWMutex
	filePath string
	log      *logger.Logger
}

// NewManager creates a new user manager
func NewManager(filePath string, log *logger.Logger) (*Manager, error) {
	mgr := &Manager{
		users:    make(map[string]*User),
		filePath: filePath,
		log:      log,
	}

	// Load existing users
	if err := mgr.load(); err != nil {
		// File doesn't exist yet - that's OK
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load users: %w", err)
		}
	}

	return mgr, nil
}

// AddUser adds a new user
func (m *Manager) AddUser(user *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[user.ID]; exists {
		return fmt.Errorf("user %q already exists", user.ID)
	}

	if user.CreatedAt == "" {
		user.CreatedAt = time.Now().Format(time.RFC3339)
	}

	m.users[user.ID] = user
	m.log.Info("User added: %s (%s)", user.ID, user.Name)

	return m.save()
}

// RemoveUser removes a user
func (m *Manager) RemoveUser(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[id]; !exists {
		return fmt.Errorf("user %q not found", id)
	}

	delete(m.users, id)
	m.log.Info("User removed: %s", id)

	return m.save()
}

// GetUser returns a user by ID
func (m *Manager) GetUser(id string) (*User, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, exists := m.users[id]
	return user, exists
}

// GetUserByShortID finds a user by their REALITY short ID
func (m *Manager) GetUserByShortID(shortID string) (*User, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, user := range m.users {
		if user.ShortID == shortID {
			return user, true
		}
	}
	return nil, false
}

// Authenticate checks if a user can connect
func (m *Manager) Authenticate(id string) (bool, string) {
	user, exists := m.GetUser(id)
	if !exists {
		return false, "user not found"
	}

	if !user.Enabled {
		return false, "user disabled"
	}

	// Check expiry
	if user.ExpiresAt != "" {
		expiry, err := time.Parse(time.RFC3339, user.ExpiresAt)
		if err == nil && time.Now().After(expiry) {
			return false, "user expired"
		}
	}

	// Check traffic limit
	if user.MaxTraffic > 0 {
		totalTraffic := user.BytesUp.Load() + user.BytesDown.Load()
		if totalTraffic >= user.MaxTraffic {
			return false, "traffic limit exceeded"
		}
	}

	return true, ""
}

// RecordTraffic records traffic for a user
func (m *Manager) RecordTraffic(id string, bytesUp, bytesDown int64) {
	user, exists := m.GetUser(id)
	if !exists {
		return
	}

	user.BytesUp.Add(bytesUp)
	user.BytesDown.Add(bytesDown)
	user.LastSeen = time.Now()
}

// ListUsers returns all users
func (m *Manager) ListUsers() []*User {
	m.mu.RLock()
	defer m.mu.RUnlock()

	users := make([]*User, 0, len(m.users))
	for _, u := range m.users {
		users = append(users, u)
	}
	return users
}

// GetAllShortIDs returns all active short IDs
func (m *Manager) GetAllShortIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0)
	for _, u := range m.users {
		if u.Enabled && u.ShortID != "" {
			ids = append(ids, u.ShortID)
		}
	}
	return ids
}

// Stats returns user statistics
type UserStats struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	BytesUp     int64  `json:"bytes_up"`
	BytesDown   int64  `json:"bytes_down"`
	Connections int64  `json:"connections"`
	LastSeen    string `json:"last_seen"`
	Enabled     bool   `json:"enabled"`
}

// GetStats returns stats for all users
func (m *Manager) GetStats() []UserStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]UserStats, 0, len(m.users))
	for _, u := range m.users {
		lastSeen := ""
		if !u.LastSeen.IsZero() {
			lastSeen = u.LastSeen.Format(time.RFC3339)
		}
		stats = append(stats, UserStats{
			ID:          u.ID,
			Name:        u.Name,
			BytesUp:     u.BytesUp.Load(),
			BytesDown:   u.BytesDown.Load(),
			Connections: u.Connections.Load(),
			LastSeen:    lastSeen,
			Enabled:     u.Enabled,
		})
	}
	return stats
}

// load reads users from file
func (m *Manager) load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return err
	}

	var users []*User
	if err := json.Unmarshal(data, &users); err != nil {
		return fmt.Errorf("failed to parse users file: %w", err)
	}

	for _, u := range users {
		m.users[u.ID] = u
	}

	m.log.Info("Loaded %d users", len(m.users))
	return nil
}

// save writes users to file
func (m *Manager) save() error {
	users := make([]*User, 0, len(m.users))
	for _, u := range m.users {
		users = append(users, u)
	}

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.filePath, data, 0600)
}
