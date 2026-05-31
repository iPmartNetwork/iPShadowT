package tun

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// SplitTunnel implements rule-based routing
// Traffic matching rules goes through the tunnel, rest goes direct
type SplitTunnel struct {
	log       *logger.Logger
	rules     []Rule
	mu        sync.RWMutex
	mode      SplitMode
	irCIDRs   []*net.IPNet // Iran IP ranges (for bypass)
	customIPs []*net.IPNet // Custom bypass ranges
}

// SplitMode defines the split tunneling mode
type SplitMode string

const (
	// ModeAll routes all traffic through tunnel (default)
	ModeAll SplitMode = "all"
	// ModeBypassIR routes Iran IPs directly, rest through tunnel
	ModeBypassIR SplitMode = "bypass_ir"
	// ModeOnlyForeign only tunnels traffic to foreign IPs
	ModeOnlyForeign SplitMode = "only_foreign"
	// ModeCustom uses custom rules
	ModeCustom SplitMode = "custom"
)

// RuleAction defines what to do with matching traffic
type RuleAction string

const (
	ActionTunnel RuleAction = "tunnel" // Route through tunnel
	ActionDirect RuleAction = "direct" // Route directly
	ActionBlock  RuleAction = "block"  // Drop traffic
)

// Rule defines a routing rule
type Rule struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"`     // "ip", "cidr", "domain", "port", "app"
	Value    string     `json:"value"`    // The match value
	Action   RuleAction `json:"action"`
	Priority int        `json:"priority"` // Lower = higher priority
}

// SplitConfig configures split tunneling
type SplitConfig struct {
	Mode       SplitMode
	Rules      []Rule
	IRListFile string // Path to Iran IP list file
	BypassLAN  bool   // Always bypass local networks
}

// NewSplitTunnel creates a new split tunnel router
func NewSplitTunnel(cfg SplitConfig, log *logger.Logger) *SplitTunnel {
	st := &SplitTunnel{
		log:  log,
		mode: cfg.Mode,
	}

	if cfg.Mode == "" {
		st.mode = ModeAll
	}

	// Load rules
	st.rules = cfg.Rules

	// Load Iran IP ranges if needed
	if cfg.Mode == ModeBypassIR || cfg.Mode == ModeOnlyForeign {
		if cfg.IRListFile != "" {
			st.loadIRList(cfg.IRListFile)
		} else {
			st.loadDefaultIRRanges()
		}
	}

	// Add LAN bypass rules
	if cfg.BypassLAN {
		st.addLANBypass()
	}

	log.Info("Split tunnel: mode=%s, rules=%d, IR ranges=%d", st.mode, len(st.rules), len(st.irCIDRs))
	return st
}

// ShouldTunnel determines if traffic to the given destination should go through the tunnel
func (st *SplitTunnel) ShouldTunnel(destIP net.IP, destPort int, domain string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()

	switch st.mode {
	case ModeAll:
		return true

	case ModeBypassIR:
		// If destination is in Iran, go direct
		if st.isIranIP(destIP) {
			return false
		}
		// If destination is local, go direct
		if st.isLocalIP(destIP) {
			return false
		}
		return true

	case ModeOnlyForeign:
		// Only tunnel if NOT Iran and NOT local
		if st.isIranIP(destIP) || st.isLocalIP(destIP) {
			return false
		}
		return true

	case ModeCustom:
		return st.evaluateRules(destIP, destPort, domain)

	default:
		return true
	}
}

// evaluateRules checks custom rules in priority order
func (st *SplitTunnel) evaluateRules(destIP net.IP, destPort int, domain string) bool {
	for _, rule := range st.rules {
		if st.matchRule(rule, destIP, destPort, domain) {
			switch rule.Action {
			case ActionTunnel:
				return true
			case ActionDirect:
				return false
			case ActionBlock:
				return false // Block = don't tunnel (connection will fail)
			}
		}
	}
	// Default: tunnel everything
	return true
}

// matchRule checks if a rule matches the given destination
func (st *SplitTunnel) matchRule(rule Rule, destIP net.IP, destPort int, domain string) bool {
	switch rule.Type {
	case "ip":
		ruleIP := net.ParseIP(rule.Value)
		return ruleIP != nil && ruleIP.Equal(destIP)

	case "cidr":
		_, cidr, err := net.ParseCIDR(rule.Value)
		if err != nil {
			return false
		}
		return cidr.Contains(destIP)

	case "domain":
		if domain == "" {
			return false
		}
		// Support wildcard: *.example.com
		if strings.HasPrefix(rule.Value, "*.") {
			suffix := rule.Value[1:] // .example.com
			return strings.HasSuffix(domain, suffix) || domain == rule.Value[2:]
		}
		return domain == rule.Value

	case "port":
		// Parse port or port range
		return matchPort(rule.Value, destPort)

	default:
		return false
	}
}

// isIranIP checks if an IP is in Iran's IP ranges
func (st *SplitTunnel) isIranIP(ip net.IP) bool {
	for _, cidr := range st.irCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// isLocalIP checks if an IP is a local/private address
func (st *SplitTunnel) isLocalIP(ip net.IP) bool {
	for _, cidr := range st.customIPs {
		if cidr.Contains(ip) {
			return true
		}
	}

	// Standard private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fe80::/10",
	}

	for _, r := range privateRanges {
		_, cidr, _ := net.ParseCIDR(r)
		if cidr != nil && cidr.Contains(ip) {
			return true
		}
	}

	return false
}

// loadIRList loads Iran IP ranges from a file (one CIDR per line)
func (st *SplitTunnel) loadIRList(path string) {
	file, err := os.Open(path)
	if err != nil {
		st.log.Warn("Split tunnel: failed to load IR list from %s: %v", path, err)
		st.loadDefaultIRRanges()
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, cidr, err := net.ParseCIDR(line)
		if err == nil {
			st.irCIDRs = append(st.irCIDRs, cidr)
		}
	}

	st.log.Info("Split tunnel: loaded %d Iran IP ranges from %s", len(st.irCIDRs), path)
}

// loadDefaultIRRanges loads common Iran IP ranges
func (st *SplitTunnel) loadDefaultIRRanges() {
	// Major Iran IP ranges (AS12880, AS44244, AS197207, etc.)
	iranRanges := []string{
		"2.144.0.0/14",
		"2.176.0.0/12",
		"5.22.0.0/17",
		"5.52.0.0/15",
		"5.56.128.0/17",
		"5.57.32.0/21",
		"5.61.24.0/21",
		"5.62.160.0/19",
		"5.63.8.0/21",
		"5.72.0.0/13",
		"5.104.208.0/21",
		"5.106.0.0/16",
		"5.112.0.0/12",
		"5.134.128.0/18",
		"5.144.128.0/21",
		"5.145.112.0/21",
		"5.160.0.0/16",
		"5.190.0.0/16",
		"5.198.160.0/19",
		"5.200.64.0/18",
		"5.201.128.0/17",
		"5.202.0.0/16",
		"5.208.0.0/12",
		"31.2.0.0/17",
		"31.7.64.0/21",
		"31.14.80.0/20",
		"31.24.200.0/21",
		"31.40.0.0/18",
		"31.56.0.0/14",
		"37.10.64.0/22",
		"37.19.0.0/17",
		"37.32.0.0/14",
		"37.44.56.0/21",
		"37.63.128.0/17",
		"37.98.0.0/17",
		"37.114.192.0/18",
		"37.128.128.0/17",
		"37.129.0.0/16",
		"37.130.200.0/21",
		"37.137.0.0/16",
		"37.148.0.0/17",
		"37.152.160.0/19",
		"37.153.128.0/18",
		"37.156.0.0/16",
		"37.191.64.0/19",
		"37.202.128.0/17",
		"37.221.0.0/18",
		"37.228.131.0/24",
		"37.235.16.0/20",
		"37.254.0.0/16",
		"46.18.248.0/21",
		"46.21.80.0/20",
		"46.28.72.0/21",
		"46.32.0.0/19",
		"46.34.96.0/19",
		"46.36.96.0/20",
		"46.38.128.0/17",
		"46.41.192.0/18",
		"46.51.0.0/17",
		"46.62.128.0/17",
		"46.100.0.0/16",
		"46.143.0.0/17",
		"46.148.32.0/20",
		"46.164.0.0/16",
		"46.167.128.0/19",
		"46.182.32.0/21",
		"46.209.0.0/16",
		"46.224.0.0/15",
		"46.235.76.0/23",
		"46.245.0.0/17",
		"46.248.32.0/19",
		"46.249.96.0/24",
		"46.251.224.0/24",
		"46.255.216.0/21",
	}

	for _, r := range iranRanges {
		_, cidr, err := net.ParseCIDR(r)
		if err == nil {
			st.irCIDRs = append(st.irCIDRs, cidr)
		}
	}
}

// addLANBypass adds local network bypass rules
func (st *SplitTunnel) addLANBypass() {
	lanRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
	}

	for _, r := range lanRanges {
		_, cidr, _ := net.ParseCIDR(r)
		if cidr != nil {
			st.customIPs = append(st.customIPs, cidr)
		}
	}
}

// AddRule adds a routing rule dynamically
func (st *SplitTunnel) AddRule(rule Rule) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.rules = append(st.rules, rule)
}

// RemoveRule removes a rule by name
func (st *SplitTunnel) RemoveRule(name string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, r := range st.rules {
		if r.Name == name {
			st.rules = append(st.rules[:i], st.rules[i+1:]...)
			return
		}
	}
}

// GetRules returns all current rules
func (st *SplitTunnel) GetRules() []Rule {
	st.mu.RLock()
	defer st.mu.RUnlock()
	result := make([]Rule, len(st.rules))
	copy(result, st.rules)
	return result
}

// matchPort checks if a port matches a port specification (single port or range)
func matchPort(spec string, port int) bool {
	// Try single port
	var p int
	if _, err := fmt.Sscanf(spec, "%d", &p); err == nil {
		return p == port
	}

	// Try range: "80-443"
	var low, high int
	if _, err := fmt.Sscanf(spec, "%d-%d", &low, &high); err == nil {
		return port >= low && port <= high
	}

	return false
}
