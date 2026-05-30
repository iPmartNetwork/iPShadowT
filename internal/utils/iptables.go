package utils

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// IPTables manages iptables rules for the tunnel
type IPTables struct {
	log   *logger.Logger
	rules []iptRule // Track added rules for cleanup
}

type iptRule struct {
	table string
	chain string
	args  []string
}

// NewIPTables creates a new iptables manager
func NewIPTables(log *logger.Logger) *IPTables {
	return &IPTables{
		log:   log,
		rules: make([]iptRule, 0),
	}
}

// SetupNAT configures NAT/masquerade for the tunnel interface
func (ipt *IPTables) SetupNAT(tunInterface, outInterface string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	// Enable masquerade on outgoing interface
	if err := ipt.addRule("nat", "POSTROUTING", "-o", outInterface, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("failed to add MASQUERADE rule: %w", err)
	}

	// Allow forwarding from TUN to outgoing
	if err := ipt.addRule("filter", "FORWARD", "-i", tunInterface, "-o", outInterface, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add FORWARD rule (out): %w", err)
	}

	// Allow forwarding from outgoing to TUN (established connections)
	if err := ipt.addRule("filter", "FORWARD", "-i", outInterface, "-o", tunInterface, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add FORWARD rule (in): %w", err)
	}

	ipt.log.Info("NAT configured: %s → %s", tunInterface, outInterface)
	return nil
}

// SetupTransparentProxy configures transparent proxy redirect
func (ipt *IPTables) SetupTransparentProxy(listenPort int, excludeIPs []string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	// Create custom chain
	ipt.runIPT("nat", "-N", "IPSHADOWT")

	// Exclude local networks
	localNets := []string{
		"0.0.0.0/8", "10.0.0.0/8", "127.0.0.0/8",
		"169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16",
		"224.0.0.0/4", "240.0.0.0/4",
	}

	for _, net := range localNets {
		ipt.runIPT("nat", "-A", "IPSHADOWT", "-d", net, "-j", "RETURN")
	}

	// Exclude specific IPs (e.g., server IP)
	for _, ip := range excludeIPs {
		ipt.runIPT("nat", "-A", "IPSHADOWT", "-d", ip, "-j", "RETURN")
	}

	// Redirect TCP to local proxy
	portStr := fmt.Sprintf("%d", listenPort)
	if err := ipt.addRule("nat", "IPSHADOWT", "-p", "tcp", "-j", "REDIRECT", "--to-ports", portStr); err != nil {
		return err
	}

	// Apply to OUTPUT chain
	if err := ipt.addRule("nat", "OUTPUT", "-p", "tcp", "-j", "IPSHADOWT"); err != nil {
		return err
	}

	ipt.log.Info("Transparent proxy configured (port: %d)", listenPort)
	return nil
}

// SetupPortForward configures port forwarding via iptables
func (ipt *IPTables) SetupPortForward(srcPort int, dstAddr string, dstPort int, protocol string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	if protocol == "" {
		protocol = "tcp"
	}

	// DNAT rule
	dest := fmt.Sprintf("%s:%d", dstAddr, dstPort)
	if err := ipt.addRule("nat", "PREROUTING",
		"-p", protocol,
		"--dport", fmt.Sprintf("%d", srcPort),
		"-j", "DNAT", "--to-destination", dest); err != nil {
		return err
	}

	ipt.log.Info("Port forward: :%d → %s (%s)", srcPort, dest, protocol)
	return nil
}

// Cleanup removes all rules added by this instance
func (ipt *IPTables) Cleanup() {
	if runtime.GOOS != "linux" {
		return
	}

	// Remove rules in reverse order
	for i := len(ipt.rules) - 1; i >= 0; i-- {
		rule := ipt.rules[i]
		args := []string{"-t", rule.table, "-D", rule.chain}
		args = append(args, rule.args...)
		ipt.runIPT(rule.table, append([]string{"-D", rule.chain}, rule.args...)...)
	}

	// Remove custom chain
	ipt.runIPT("nat", "-F", "IPSHADOWT")
	ipt.runIPT("nat", "-X", "IPSHADOWT")

	ipt.rules = nil
	ipt.log.Info("iptables rules cleaned up")
}

// addRule adds an iptables rule and tracks it for cleanup
func (ipt *IPTables) addRule(table, chain string, args ...string) error {
	fullArgs := append([]string{"-A", chain}, args...)
	if err := ipt.runIPT(table, fullArgs...); err != nil {
		return err
	}

	ipt.rules = append(ipt.rules, iptRule{
		table: table,
		chain: chain,
		args:  args,
	})

	return nil
}

// runIPT executes an iptables command
func (ipt *IPTables) runIPT(table string, args ...string) error {
	fullArgs := append([]string{"-t", table}, args...)
	cmd := exec.Command("iptables", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s: %s (%w)", strings.Join(fullArgs, " "), strings.TrimSpace(string(output)), err)
	}
	return nil
}
