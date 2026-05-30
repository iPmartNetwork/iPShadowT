package tunnel

import (
	"net"
	"strings"
	"sync"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// SplitTunnel implements split tunneling
// Only specified traffic goes through the tunnel, rest goes direct
type SplitTunnel struct {
	log       *logger.Logger
	rules     []SplitRule
	mu        sync.RWMutex
	geoIPData map[string]string // IP → country code cache
}

// SplitRule defines a split tunneling rule
type SplitRule struct {
	Type    string // "domain", "ip", "cidr", "geoip"
	Value   string // The pattern to match
	Action  string // "proxy" or "direct"
}

// NewSplitTunnel creates a new split tunnel manager
func NewSplitTunnel(log *logger.Logger) *SplitTunnel {
	return &SplitTunnel{
		log:       log,
		rules:     make([]SplitRule, 0),
		geoIPData: make(map[string]string),
	}
}

// AddRule adds a split tunneling rule
func (st *SplitTunnel) AddRule(rule SplitRule) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.rules = append(st.rules, rule)
}

// LoadDefaultIranRules loads default rules for Iran
// Iranian IPs and domains go direct, everything else through tunnel
func (st *SplitTunnel) LoadDefaultIranRules() {
	// Iranian IP ranges (major ISPs)
	iranCIDRs := []string{
		"2.144.0.0/14", "2.176.0.0/12", "5.22.0.0/17",
		"5.52.0.0/15", "5.160.0.0/16", "5.200.0.0/16",
		"31.2.0.0/17", "31.56.0.0/14", "37.32.0.0/14",
		"37.114.192.0/18", "37.137.0.0/16", "37.148.0.0/17",
		"37.156.0.0/16", "46.18.248.0/21", "46.21.80.0/21",
		"46.34.160.0/19", "46.36.96.0/20", "46.38.128.0/17",
		"46.41.192.0/18", "46.62.128.0/17", "46.100.0.0/16",
		"46.143.0.0/17", "46.164.0.0/16", "46.182.32.0/19",
		"46.209.0.0/16", "46.224.0.0/15", "46.235.0.0/17",
		"46.245.0.0/17", "46.249.96.0/21", "46.251.224.0/24",
		"62.60.128.0/17", "62.102.128.0/20", "62.220.96.0/19",
		"65.20.0.0/17", "66.79.96.0/19", "69.194.64.0/18",
		"77.36.128.0/17", "77.81.128.0/20", "77.104.64.0/18",
		"78.38.0.0/15", "78.109.192.0/20", "78.111.0.0/20",
		"80.66.176.0/20", "80.71.112.0/20", "80.75.0.0/20",
		"80.191.0.0/16", "80.210.0.0/16", "80.253.128.0/19",
		"81.12.0.0/17", "81.16.112.0/20", "81.28.32.0/19",
		"81.29.240.0/20", "81.31.160.0/19", "81.90.144.0/20",
		"81.91.128.0/17", "81.92.216.0/21",
		"82.99.192.0/18", "82.138.140.0/25",
		"83.120.0.0/14", "83.147.192.0/18",
		"84.47.192.0/18", "84.241.0.0/18",
		"85.9.64.0/18", "85.15.0.0/17", "85.133.0.0/16",
		"85.185.0.0/16", "85.198.48.0/20",
		"86.55.0.0/16", "86.104.32.0/20", "86.106.142.0/24",
		"86.107.0.0/20", "86.109.32.0/19",
		"87.107.0.0/16", "87.236.208.0/21", "87.247.168.0/21",
		"88.131.240.0/20", "88.135.32.0/20",
		"89.32.0.0/18", "89.34.32.0/19", "89.34.128.0/18",
		"89.38.80.0/20", "89.39.208.0/20", "89.42.32.0/21",
		"89.42.208.0/22", "89.43.0.0/21", "89.165.0.0/17",
		"89.196.0.0/16", "89.219.64.0/18", "89.235.64.0/18",
		"91.92.104.0/21", "91.98.0.0/16", "91.99.0.0/16",
		"91.106.64.0/19", "91.107.128.0/21", "91.108.128.0/18",
		"91.109.104.0/21", "91.133.128.0/17",
		"91.184.64.0/19", "91.185.128.0/19",
		"91.186.192.0/19", "91.187.0.0/19",
		"91.199.9.0/24", "91.209.96.0/21",
		"91.220.79.0/24", "91.220.243.0/24",
		"91.222.196.0/22", "91.224.20.0/23",
		"91.225.52.0/22", "91.226.0.0/22",
		"91.227.84.0/22", "91.228.22.0/23",
		"91.229.46.0/23", "91.232.64.0/22",
		"91.236.168.0/23", "91.237.254.0/23",
		"91.238.0.0/21", "91.239.14.0/24",
		"91.240.60.0/22", "91.240.180.0/22",
		"91.241.20.0/22", "91.242.44.0/22",
		"91.243.126.0/23", "91.244.120.0/22",
		"91.245.228.0/22", "91.247.171.0/24",
		"91.247.174.0/24", "91.248.0.0/21",
		"91.250.224.0/20", "91.251.0.0/16",
		"92.42.48.0/21", "92.43.160.0/22",
		"92.61.176.0/20", "92.114.16.0/20",
		"93.110.0.0/16", "93.113.224.0/20",
		"93.114.16.0/20", "93.115.224.0/21",
		"93.117.0.0/16", "93.118.96.0/19",
		"93.119.32.0/19", "93.126.0.0/18",
		"94.24.0.0/17", "94.74.128.0/17",
		"94.101.128.0/20", "94.139.160.0/19",
		"94.176.8.0/21", "94.177.64.0/18",
		"94.182.0.0/15", "94.184.0.0/16",
		"94.199.136.0/22",
		"95.38.0.0/16", "95.64.0.0/17",
		"95.80.128.0/18", "95.81.64.0/18",
		"95.142.224.0/20", "95.156.220.0/22",
		"95.162.0.0/16",
	}

	for _, cidr := range iranCIDRs {
		st.AddRule(SplitRule{Type: "cidr", Value: cidr, Action: "direct"})
	}

	// Iranian domains go direct
	iranDomains := []string{".ir", ".co.ir", ".ac.ir", ".sch.ir", ".gov.ir"}
	for _, d := range iranDomains {
		st.AddRule(SplitRule{Type: "domain", Value: d, Action: "direct"})
	}

	st.log.Info("Loaded %d Iran split-tunnel rules", len(st.rules))
}

// ShouldProxy determines if a destination should go through the tunnel
// Returns true if traffic should be proxied, false if direct
func (st *SplitTunnel) ShouldProxy(dest string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if len(st.rules) == 0 {
		return true // No rules = proxy everything
	}

	host, _, _ := net.SplitHostPort(dest)
	if host == "" {
		host = dest
	}

	for _, rule := range st.rules {
		if st.matchRule(rule, host) {
			return rule.Action == "proxy"
		}
	}

	// Default: proxy
	return true
}

// matchRule checks if a host matches a rule
func (st *SplitTunnel) matchRule(rule SplitRule, host string) bool {
	switch rule.Type {
	case "domain":
		return strings.HasSuffix(host, rule.Value) || host == strings.TrimPrefix(rule.Value, ".")

	case "ip":
		return host == rule.Value

	case "cidr":
		_, network, err := net.ParseCIDR(rule.Value)
		if err != nil {
			return false
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		return network.Contains(ip)

	default:
		return false
	}
}
