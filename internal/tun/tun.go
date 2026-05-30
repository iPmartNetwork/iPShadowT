package tun

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Device represents a TUN network device
// TUN captures all system traffic and routes it through the tunnel
type Device struct {
	name    string
	mtu     int
	addr    string // e.g., "10.0.0.2/24"
	gateway string // e.g., "10.0.0.1"
	log     *logger.Logger
	fd      int
	running bool
}

// Config holds TUN device configuration
type Config struct {
	Name    string // Device name (e.g., "ipshadowt0")
	MTU     int    // MTU size (default: 1500)
	Address string // TUN IP address (e.g., "10.0.0.2/24")
	Gateway string // Gateway IP (e.g., "10.0.0.1")
	DNS     string // DNS server to use (e.g., "1.1.1.1")
}

// NewDevice creates a new TUN device
func NewDevice(cfg Config, log *logger.Logger) (*Device, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("TUN device only supported on Linux")
	}

	if cfg.Name == "" {
		cfg.Name = "ipshadowt0"
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1500
	}
	if cfg.Address == "" {
		cfg.Address = "10.0.0.2/24"
	}
	if cfg.Gateway == "" {
		cfg.Gateway = "10.0.0.1"
	}

	return &Device{
		name:    cfg.Name,
		mtu:     cfg.MTU,
		addr:    cfg.Address,
		gateway: cfg.Gateway,
		log:     log,
	}, nil
}

// Create creates the TUN device using ip commands
func (d *Device) Create() error {
	// Create TUN device
	if err := runCmd("ip", "tuntap", "add", "dev", d.name, "mode", "tun"); err != nil {
		return fmt.Errorf("failed to create TUN device: %w", err)
	}

	// Set MTU
	if err := runCmd("ip", "link", "set", "dev", d.name, "mtu", fmt.Sprintf("%d", d.mtu)); err != nil {
		d.Destroy()
		return fmt.Errorf("failed to set MTU: %w", err)
	}

	// Assign IP address
	if err := runCmd("ip", "addr", "add", d.addr, "dev", d.name); err != nil {
		d.Destroy()
		return fmt.Errorf("failed to assign address: %w", err)
	}

	// Bring up the device
	if err := runCmd("ip", "link", "set", "dev", d.name, "up"); err != nil {
		d.Destroy()
		return fmt.Errorf("failed to bring up device: %w", err)
	}

	d.running = true
	d.log.Info("TUN device %s created (addr: %s, mtu: %d)", d.name, d.addr, d.mtu)
	return nil
}

// SetupRouting configures routing to send traffic through the TUN device
func (d *Device) SetupRouting(serverIP string, excludeNets []string) error {
	// Get current default gateway
	defaultGW, defaultIface, err := getDefaultGateway()
	if err != nil {
		return fmt.Errorf("failed to get default gateway: %w", err)
	}

	d.log.Debug("Default gateway: %s via %s", defaultGW, defaultIface)

	// Add route to server IP via original gateway (so tunnel traffic doesn't loop)
	if serverIP != "" {
		if err := runCmd("ip", "route", "add", serverIP+"/32", "via", defaultGW, "dev", defaultIface); err != nil {
			d.log.Warn("Failed to add server route: %v", err)
		}
	}

	// Add excluded networks via original gateway
	for _, net := range excludeNets {
		if err := runCmd("ip", "route", "add", net, "via", defaultGW, "dev", defaultIface); err != nil {
			d.log.Warn("Failed to add exclude route for %s: %v", net, err)
		}
	}

	// Replace default route to go through TUN
	if err := runCmd("ip", "route", "replace", "default", "via", d.gateway, "dev", d.name); err != nil {
		return fmt.Errorf("failed to set default route: %w", err)
	}

	d.log.Info("Routing configured: all traffic → %s", d.name)
	return nil
}

// Destroy removes the TUN device and restores routing
func (d *Device) Destroy() error {
	if !d.running {
		return nil
	}

	// Remove the TUN device (this also removes associated routes)
	if err := runCmd("ip", "link", "del", d.name); err != nil {
		d.log.Warn("Failed to delete TUN device: %v", err)
	}

	d.running = false
	d.log.Info("TUN device %s destroyed", d.name)
	return nil
}

// Name returns the device name
func (d *Device) Name() string {
	return d.name
}

// getDefaultGateway returns the current default gateway and interface
func getDefaultGateway() (gateway, iface string, err error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", err
	}

	// Parse: "default via 192.168.1.1 dev eth0 ..."
	parts := strings.Fields(string(out))
	for i, p := range parts {
		if p == "via" && i+1 < len(parts) {
			gateway = parts[i+1]
		}
		if p == "dev" && i+1 < len(parts) {
			iface = parts[i+1]
		}
	}

	if gateway == "" || iface == "" {
		return "", "", fmt.Errorf("could not parse default route: %s", string(out))
	}

	return gateway, iface, nil
}

// GetLocalNetworks returns local network CIDRs to exclude from tunnel
func GetLocalNetworks() []string {
	excludes := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"255.255.255.255/32",
	}

	// Also add local interface networks
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
					excludes = append(excludes, ipNet.String())
				}
			}
		}
	}

	return excludes
}

// runCmd executes a command and returns error if it fails
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s (%w)", name, strings.Join(args, " "), string(output), err)
	}
	return nil
}
