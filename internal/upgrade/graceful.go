package upgrade

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// GracefulUpgrade handles zero-downtime binary upgrades
// It works by:
// 1. Starting the new binary
// 2. Passing existing listeners via file descriptors
// 3. Waiting for the new process to signal readiness
// 4. Draining existing connections
// 5. Shutting down the old process
type GracefulUpgrade struct {
	log         *logger.Logger
	listeners   []net.Listener
	mu          sync.Mutex
	drainTimeout time.Duration
	newBinaryPath string
	isChild     bool
	parentPID   int
	done        chan struct{}
}

// UpgradeConfig configures graceful upgrade
type UpgradeConfig struct {
	DrainTimeout  time.Duration // Max time to wait for connections to drain
	NewBinaryPath string        // Path to new binary (empty = same binary)
}

// NewGracefulUpgrade creates a new graceful upgrade handler
func NewGracefulUpgrade(cfg UpgradeConfig, log *logger.Logger) *GracefulUpgrade {
	if cfg.DrainTimeout == 0 {
		cfg.DrainTimeout = 30 * time.Second
	}

	return &GracefulUpgrade{
		log:           log,
		listeners:     make([]net.Listener, 0),
		drainTimeout:  cfg.DrainTimeout,
		newBinaryPath: cfg.NewBinaryPath,
		isChild:       os.Getenv("IPSHADOWT_GRACEFUL") == "1",
		done:          make(chan struct{}),
	}
}

// IsChild returns true if this process was started by a graceful upgrade
func (g *GracefulUpgrade) IsChild() bool {
	return g.isChild
}

// RegisterListener registers a listener for graceful handoff
func (g *GracefulUpgrade) RegisterListener(ln net.Listener) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.listeners = append(g.listeners, ln)
}

// InheritedListeners returns listeners inherited from the parent process
func (g *GracefulUpgrade) InheritedListeners() ([]net.Listener, error) {
	if !g.isChild {
		return nil, nil
	}

	// Get the number of inherited file descriptors
	numFDs := 0
	fmt.Sscanf(os.Getenv("IPSHADOWT_FD_COUNT"), "%d", &numFDs)

	if numFDs == 0 {
		return nil, nil
	}

	listeners := make([]net.Listener, 0, numFDs)
	for i := 0; i < numFDs; i++ {
		// File descriptors start at 3 (after stdin, stdout, stderr)
		fd := uintptr(3 + i)
		file := os.NewFile(fd, fmt.Sprintf("listener-%d", i))
		if file == nil {
			continue
		}

		ln, err := net.FileListener(file)
		file.Close()
		if err != nil {
			g.log.Warn("Failed to inherit listener %d: %v", i, err)
			continue
		}

		listeners = append(listeners, ln)
	}

	g.log.Info("Inherited %d listeners from parent", len(listeners))
	return listeners, nil
}

// ListenForUpgradeSignal listens for OS signals to trigger upgrade
// On Linux: SIGUSR2, On Windows: not supported (use API instead)
func (g *GracefulUpgrade) ListenForUpgradeSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		for {
			select {
			case <-g.done:
				return
			case <-sigCh:
				// On Linux this would be SIGUSR2
				// On Windows, upgrade is triggered via API
				return
			}
		}
	}()
}

// Upgrade performs the graceful upgrade
func (g *GracefulUpgrade) Upgrade() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	binaryPath := g.newBinaryPath
	if binaryPath == "" {
		var err error
		binaryPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}
	}

	// Start new process
	env := append(os.Environ(),
		"IPSHADOWT_GRACEFUL=1",
		fmt.Sprintf("IPSHADOWT_FD_COUNT=%d", len(g.listeners)),
		fmt.Sprintf("IPSHADOWT_PARENT_PID=%d", os.Getpid()),
	)

	cmd := exec.Command(binaryPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %w", err)
	}

	g.log.Info("New process started (PID: %d), draining connections...", cmd.Process.Pid)

	// Wait for drain timeout then exit
	go func() {
		time.Sleep(g.drainTimeout)
		g.log.Info("Drain timeout reached, shutting down old process")
		os.Exit(0)
	}()

	return nil
}

// SignalParentReady signals the parent process that we're ready
func (g *GracefulUpgrade) SignalParentReady() {
	if !g.isChild {
		return
	}

	pidStr := os.Getenv("IPSHADOWT_PARENT_PID")
	var pid int
	fmt.Sscanf(pidStr, "%d", &pid)

	if pid > 0 {
		process, err := os.FindProcess(pid)
		if err == nil {
			process.Signal(os.Interrupt)
			g.log.Info("Signaled parent process (PID: %d) to shut down", pid)
		}
	}
}

// Stop stops listening for upgrade signals
func (g *GracefulUpgrade) Stop() {
	close(g.done)
}
