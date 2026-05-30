package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Manager handles backup and restore operations
type Manager struct {
	configDir string // Directory containing config files (e.g., /etc/ipshadowt/)
	backupDir string // Directory to store backups (e.g., /etc/ipshadowt/backups/)
	maxBackups int   // Maximum number of backups to keep
	log       *logger.Logger
}

// Config holds backup configuration
type Config struct {
	ConfigDir  string // Path to config directory
	BackupDir  string // Path to backup storage
	MaxBackups int    // Max backups to retain (0 = unlimited)
	AutoBackup bool   // Enable automatic periodic backups
	Interval   int    // Auto-backup interval in hours
}

// NewManager creates a new backup manager
func NewManager(cfg Config, log *logger.Logger) (*Manager, error) {
	if cfg.ConfigDir == "" {
		cfg.ConfigDir = "/etc/ipshadowt"
	}
	if cfg.BackupDir == "" {
		cfg.BackupDir = filepath.Join(cfg.ConfigDir, "backups")
	}
	if cfg.MaxBackups == 0 {
		cfg.MaxBackups = 10
	}

	// Create backup directory if it doesn't exist
	if err := os.MkdirAll(cfg.BackupDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create backup dir: %w", err)
	}

	return &Manager{
		configDir:  cfg.ConfigDir,
		backupDir:  cfg.BackupDir,
		maxBackups: cfg.MaxBackups,
		log:        log,
	}, nil
}

// Backup creates a backup of all configuration and user data
// Returns the path to the created backup file
func (m *Manager) Backup() (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("ipshadowt-backup-%s.tar.gz", timestamp)
	backupPath := filepath.Join(m.backupDir, filename)

	// Create backup file
	file, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	// Create gzip writer
	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Files to backup
	filesToBackup := []string{
		"config.toml",
		"users.json",
		"*.toml",
	}

	// Walk config directory and add matching files
	err = filepath.Walk(m.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't read
		}

		// Skip backup directory itself
		if strings.HasPrefix(path, m.backupDir) {
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file matches our patterns
		name := info.Name()
		shouldBackup := false
		for _, pattern := range filesToBackup {
			if matched, _ := filepath.Match(pattern, name); matched {
				shouldBackup = true
				break
			}
		}

		// Also backup .json files (users, etc.)
		if strings.HasSuffix(name, ".json") {
			shouldBackup = true
		}

		if !shouldBackup {
			return nil
		}

		// Add file to tar
		return m.addFileToTar(tarWriter, path, info)
	})

	if err != nil {
		os.Remove(backupPath)
		return "", fmt.Errorf("backup failed: %w", err)
	}

	// Cleanup old backups
	m.cleanupOldBackups()

	m.log.Info("✅ Backup created: %s", backupPath)
	return backupPath, nil
}

// Restore restores from a backup file
func (m *Manager) Restore(backupPath string) error {
	// Open backup file
	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer file.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("invalid backup file (not gzip): %w", err)
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	// Extract files
	restored := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		// Security: prevent path traversal
		targetPath := filepath.Join(m.configDir, filepath.Clean(header.Name))
		if !strings.HasPrefix(targetPath, m.configDir) {
			m.log.Warn("Skipping suspicious path: %s", header.Name)
			continue
		}

		// Create directories if needed
		dir := filepath.Dir(targetPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create dir %s: %w", dir, err)
		}

		// Write file
		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", targetPath, err)
		}

		if _, err := io.Copy(outFile, tarReader); err != nil {
			outFile.Close()
			return fmt.Errorf("failed to write %s: %w", targetPath, err)
		}
		outFile.Close()

		restored++
		m.log.Debug("Restored: %s", header.Name)
	}

	m.log.Info("✅ Restore complete: %d files restored from %s", restored, filepath.Base(backupPath))
	return nil
}

// ListBackups returns available backups sorted by date (newest first)
func (m *Manager) ListBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup dir: %w", err)
	}

	backups := make([]BackupInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Name:      entry.Name(),
			Path:      filepath.Join(m.backupDir, entry.Name()),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}

	// Sort newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// BackupInfo holds information about a backup
type BackupInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// addFileToTar adds a single file to the tar archive
func (m *Manager) addFileToTar(tw *tar.Writer, filePath string, info os.FileInfo) error {
	// Get relative path
	relPath, err := filepath.Rel(m.configDir, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}

	// Create tar header
	header := &tar.Header{
		Name:    relPath,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	// Write file content
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(tw, file)
	return err
}

// cleanupOldBackups removes old backups exceeding maxBackups
func (m *Manager) cleanupOldBackups() {
	backups, err := m.ListBackups()
	if err != nil {
		return
	}

	if len(backups) <= m.maxBackups {
		return
	}

	// Remove oldest backups
	for i := m.maxBackups; i < len(backups); i++ {
		os.Remove(backups[i].Path)
		m.log.Debug("Removed old backup: %s", backups[i].Name)
	}
}

// StartAutoBackup starts periodic automatic backups
func (m *Manager) StartAutoBackup(intervalHours int, done chan struct{}) {
	if intervalHours <= 0 {
		intervalHours = 24 // Default: daily
	}

	go func() {
		ticker := time.NewTicker(time.Duration(intervalHours) * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, err := m.Backup(); err != nil {
					m.log.Error("Auto-backup failed: %v", err)
				}
			}
		}
	}()

	m.log.Info("Auto-backup enabled (every %d hours, max %d backups)", intervalHours, m.maxBackups)
}
