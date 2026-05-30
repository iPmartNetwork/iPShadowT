package tun

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// TAPDevice represents a TAP (Layer 2) network device
// Unlike TUN (Layer 3, IP only), TAP works at Ethernet level
// This means it can carry:
// - ARP packets
// - Broadcast traffic
// - Non-IP protocols
// - Full Ethernet frames
//
// Use cases:
// - Bridge two networks (Iran ↔ Abroad) at Layer 2
// - Make remote server appear as if it's on the same LAN
// - Support protocols that need broadcast (DHCP, mDNS, etc.)
type TAPDevice struct {
	name    string
	mtu     int
	addr    string // IP address (e.g., "10.0.0.1/24")
	log     *logger.Logger
	running bool
}

// TAPConfig holds TAP device configuration
type TAPConfig struct {
	Name    string // Device name (e.g., "ipshadowt-tap0")
	MTU     int    // MTU size (default: 1200 for multi-layer, 1500 for direct)
	Address string // IP address with CIDR (e.g., "10.0.0.1/24")
	Bridge  string // Bridge interface to join (optional)
}

// NewTAPDevice creates a new TAP device
func NewTAPDevice(cfg TAPConfig, log *logger.Logger) (*TAPDevice, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("TAP device only supported on Linux")
	}

	if cfg.Name == "" {
		cfg.Name = "ipshadowt-tap0"
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1200 // Conservative default for multi-layer tunnels
	}
	if cfg.Address == "" {
		cfg.Address = "10.0.0.1/24"
	}

	return &TAPDevice{
		name: cfg.Name,
		mtu:  cfg.MTU,
		addr: cfg.Address,
		log:  log,
	}, nil
}

// Create creates and configures the TAP device
func (t *TAPDevice) Create() error {
	// Create TAP device
	if err := runCmd("ip", "tuntap", "add", "dev", t.name, "mode", "tap"); err != nil {
		return fmt.Errorf("failed to create TAP device: %w", err)
	}

	// Set MTU
	if err := runCmd("ip", "link", "set", "dev", t.name, "mtu", fmt.Sprintf("%d", t.mtu)); err != nil {
		t.Destroy()
		return fmt.Errorf("failed to set MTU: %w", err)
	}

	// Assign IP address
	if err := runCmd("ip", "addr", "add", t.addr, "dev", t.name); err != nil {
		t.Destroy()
		return fmt.Errorf("failed to assign address: %w", err)
	}

	// Bring up
	if err := runCmd("ip", "link", "set", "dev", t.name, "up"); err != nil {
		t.Destroy()
		return fmt.Errorf("failed to bring up TAP: %w", err)
	}

	// Disable IPv6 on TAP (reduces noise)
	runCmd("sysctl", "-w", fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6=1", t.name))

	t.running = true
	t.log.Info("TAP device %s created (addr: %s, mtu: %d)", t.name, t.addr, t.mtu)
	return nil
}

// AddToBridge adds the TAP device to a Linux bridge
func (t *TAPDevice) AddToBridge(bridgeName string) error {
	// Create bridge if it doesn't exist
	runCmd("ip", "link", "add", bridgeName, "type", "bridge")
	runCmd("ip", "link", "set", bridgeName, "up")

	// Add TAP to bridge
	if err := runCmd("ip", "link", "set", t.name, "master", bridgeName); err != nil {
		return fmt.Errorf("failed to add %s to bridge %s: %w", t.name, bridgeName, err)
	}

	t.log.Info("TAP %s added to bridge %s", t.name, bridgeName)
	return nil
}

// SetPromisc enables promiscuous mode on the TAP device
func (t *TAPDevice) SetPromisc(enabled bool) error {
	mode := "off"
	if enabled {
		mode = "on"
	}
	return runCmd("ip", "link", "set", t.name, "promisc", mode)
}

// Destroy removes the TAP device
func (t *TAPDevice) Destroy() error {
	if !t.running {
		return nil
	}

	if err := runCmd("ip", "link", "del", t.name); err != nil {
		t.log.Warn("Failed to delete TAP device: %v", err)
	}

	t.running = false
	t.log.Info("TAP device %s destroyed", t.name)
	return nil
}

// Name returns the device name
func (t *TAPDevice) Name() string {
	return t.name
}

// IsRunning returns whether the device is active
func (t *TAPDevice) IsRunning() bool {
	return t.running
}

// GetMACAddress returns the MAC address of the TAP device
func (t *TAPDevice) GetMACAddress() (string, error) {
	out, err := exec.Command("cat", fmt.Sprintf("/sys/class/net/%s/address", t.name)).Output()
	if err != nil {
		return "", err
	}
	return string(out[:17]), nil // MAC is 17 chars (xx:xx:xx:xx:xx:xx)
}
