package hotreload

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// Watcher monitors config file for changes and triggers reload
type Watcher struct {
	configPath string
	lastHash   [32]byte
	interval   time.Duration
	log        *logger.Logger
	onChange   func(*config.Config) error
	done       chan struct{}
	wg         sync.WaitGroup
}

// NewWatcher creates a new config file watcher
func NewWatcher(configPath string, interval time.Duration, log *logger.Logger, onChange func(*config.Config) error) *Watcher {
	if interval == 0 {
		interval = 10 * time.Second
	}

	return &Watcher{
		configPath: configPath,
		interval:   interval,
		log:        log,
		onChange:   onChange,
		done:       make(chan struct{}),
	}
}

// Start begins watching the config file
func (w *Watcher) Start() error {
	// Get initial hash
	hash, err := w.fileHash()
	if err != nil {
		return fmt.Errorf("failed to hash config: %w", err)
	}
	w.lastHash = hash

	w.wg.Add(1)
	go w.watchLoop()

	w.log.Info("Config hot-reload enabled (checking every %v)", w.interval)
	return nil
}

// watchLoop periodically checks for config changes
func (w *Watcher) watchLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// check compares current file hash with last known hash
func (w *Watcher) check() {
	hash, err := w.fileHash()
	if err != nil {
		w.log.Debug("Config hash error: %v", err)
		return
	}

	if hash == w.lastHash {
		return // No change
	}

	w.log.Info("Config file changed, reloading...")
	w.lastHash = hash

	// Load new config
	newCfg, err := config.Load(w.configPath)
	if err != nil {
		w.log.Error("Failed to parse new config: %v", err)
		return
	}

	// Call onChange handler
	if w.onChange != nil {
		if err := w.onChange(newCfg); err != nil {
			w.log.Error("Config reload failed: %v", err)
			return
		}
	}

	w.log.Info("✅ Config reloaded successfully")
}

// fileHash computes SHA-256 hash of the config file
func (w *Watcher) fileHash() ([32]byte, error) {
	data, err := os.ReadFile(w.configPath)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.done)
	w.wg.Wait()
}
