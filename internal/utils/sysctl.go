package utils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// KernelTuning applies performance-optimized kernel parameters (Linux only)
type KernelTuning struct {
	log *logger.Logger
}

// NewKernelTuning creates a new kernel tuning instance
func NewKernelTuning(log *logger.Logger) *KernelTuning {
	return &KernelTuning{log: log}
}

// Apply applies kernel tuning parameters
func (kt *KernelTuning) Apply(profile string) error {
	if runtime.GOOS != "linux" {
		kt.log.Debug("Kernel tuning only available on Linux, skipping")
		return nil
	}

	// Check if running as root
	if os.Geteuid() != 0 {
		kt.log.Warn("Kernel tuning requires root privileges, skipping")
		return nil
	}

	kt.log.Info("Applying kernel tuning profile: %s", profile)

	params := kt.getProfile(profile)
	applied := 0

	for key, value := range params {
		if err := kt.setSysctl(key, value); err != nil {
			kt.log.Debug("Failed to set %s=%s: %v", key, value, err)
		} else {
			applied++
		}
	}

	kt.log.Info("Applied %d/%d kernel parameters", applied, len(params))
	return nil
}

// getProfile returns sysctl parameters for a given profile
func (kt *KernelTuning) getProfile(profile string) map[string]string {
	base := map[string]string{
		// Enable IP forwarding
		"net.ipv4.ip_forward": "1",
		// TCP keepalive
		"net.ipv4.tcp_keepalive_time":    "60",
		"net.ipv4.tcp_keepalive_intvl":   "10",
		"net.ipv4.tcp_keepalive_probes":  "6",
		// TCP optimization
		"net.ipv4.tcp_fastopen":          "3",
		"net.ipv4.tcp_slow_start_after_idle": "0",
		"net.ipv4.tcp_no_metrics_save":   "1",
		// Connection tracking
		"net.core.somaxconn":             "65535",
		"net.core.netdev_max_backlog":    "65535",
		"net.ipv4.tcp_max_syn_backlog":   "65535",
	}

	switch profile {
	case "high_throughput":
		// Maximize throughput
		base["net.core.rmem_max"] = "67108864"       // 64MB
		base["net.core.wmem_max"] = "67108864"       // 64MB
		base["net.ipv4.tcp_rmem"] = "4096 1048576 67108864"
		base["net.ipv4.tcp_wmem"] = "4096 1048576 67108864"
		base["net.ipv4.tcp_window_scaling"] = "1"
		base["net.ipv4.tcp_timestamps"] = "1"
		base["net.ipv4.tcp_sack"] = "1"
		base["net.core.optmem_max"] = "65535"

	case "low_cpu":
		// Minimize CPU usage
		base["net.core.rmem_max"] = "4194304"        // 4MB
		base["net.core.wmem_max"] = "4194304"        // 4MB
		base["net.ipv4.tcp_rmem"] = "4096 87380 4194304"
		base["net.ipv4.tcp_wmem"] = "4096 65536 4194304"

	default: // "balanced"
		base["net.core.rmem_max"] = "16777216"       // 16MB
		base["net.core.wmem_max"] = "16777216"       // 16MB
		base["net.ipv4.tcp_rmem"] = "4096 524288 16777216"
		base["net.ipv4.tcp_wmem"] = "4096 524288 16777216"
		base["net.ipv4.tcp_window_scaling"] = "1"
		base["net.ipv4.tcp_timestamps"] = "1"
		base["net.ipv4.tcp_sack"] = "1"
	}

	return base
}

// setSysctl sets a single sysctl parameter
func (kt *KernelTuning) setSysctl(key, value string) error {
	// Try writing directly to /proc/sys
	path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
	if err := os.WriteFile(path, []byte(value), 0644); err == nil {
		return nil
	}

	// Fallback to sysctl command
	cmd := exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
	return cmd.Run()
}

// EnableIPForwarding enables IPv4 forwarding
func EnableIPForwarding(log *logger.Logger) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	kt := NewKernelTuning(log)
	return kt.setSysctl("net.ipv4.ip_forward", "1")
}
