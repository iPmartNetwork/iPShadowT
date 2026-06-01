package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config represents the main configuration structure
type Config struct {
	// General
	Mode      string `toml:"mode"`      // "server" or "client"
	LogLevel  string `toml:"log_level"` // "debug", "info", "warn", "error"
	Transport string `toml:"transport"` // "tcpmux", "wsmux", "reality", "h2mux", "shadowtls"

	// Network
	BindAddr   string `toml:"bind_addr"`   // Server: listen address
	RemoteAddr string `toml:"remote_addr"` // Client: server address

	// Security
	Password string `toml:"password"` // Pre-shared key for encryption
	TLSCert  string `toml:"tls_cert"` // Path to TLS certificate
	TLSKey   string `toml:"tls_key"`  // Path to TLS private key

	// Multiplexing
	Mux MuxConfig `toml:"mux"`

	// Connection Pool
	Pool PoolConfig `toml:"pool"`

	// Heartbeat
	Heartbeat HeartbeatConfig `toml:"heartbeat"`

	// Performance
	Performance PerformanceConfig `toml:"performance"`

	// Anti-DPI
	AntiDPI AntiDPIConfig `toml:"anti_dpi"`

	// Port Forwarding
	Forwards []ForwardConfig `toml:"forwards"`

	// Multi-Path (client only)
	Paths []PathConfig `toml:"paths"`

	// Health / Watchdog
	Health HealthConfig `toml:"health"`

	// CDN
	CDN CDNModeConfig `toml:"cdn"`

	// REALITY specific
	Reality RealityConfig `toml:"reality"`

	// KCP specific
	KCP KCPConfig `toml:"kcp"`
}

// MuxConfig configures the multiplexer
type MuxConfig struct {
	Enabled       bool `toml:"enabled"`
	Concurrency   int  `toml:"concurrency"`    // Number of mux sessions
	FrameSize     int  `toml:"frame_size"`     // Max frame size
	RecvBuffer    int  `toml:"recv_buffer"`    // Receive buffer size
	StreamBuffer  int  `toml:"stream_buffer"`  // Stream buffer size
	MaxStreams    int  `toml:"max_streams"`    // Max streams per session
}

// PoolConfig configures connection pooling
type PoolConfig struct {
	Size        int `toml:"size"`         // Number of connections in pool
	MaxIdle     int `toml:"max_idle"`     // Max idle connections
	IdleTimeout int `toml:"idle_timeout"` // Idle timeout in seconds
}

// HeartbeatConfig configures keepalive
type HeartbeatConfig struct {
	Enabled  bool `toml:"enabled"`
	Interval int  `toml:"interval"` // Interval in seconds
	Timeout  int  `toml:"timeout"`  // Timeout in seconds
}

// PerformanceConfig configures performance tuning
type PerformanceConfig struct {
	Nodelay       bool   `toml:"nodelay"`
	KeepAlive     int    `toml:"keepalive"`      // TCP keepalive in seconds
	SendBuffer    int    `toml:"send_buffer"`    // SO_SNDBUF
	RecvBuffer    int    `toml:"recv_buffer"`    // SO_RCVBUF
	BufferProfile string `toml:"buffer_profile"` // "low_cpu", "balanced", "high_throughput", "upload_boost"
	Workers       int    `toml:"workers"`        // Number of worker goroutines
	KernelTuning  bool   `toml:"kernel_tuning"`  // Auto-tune kernel params
}

// AntiDPIConfig configures anti-DPI features
type AntiDPIConfig struct {
	Enabled         bool   `toml:"enabled"`
	UTLSFingerprint string `toml:"utls_fingerprint"` // "chrome", "firefox", "safari", "random"
	Fragment        bool   `toml:"fragment"`          // TLS record fragmentation
	FragmentSize    string `toml:"fragment_size"`     // "50-100" bytes
	Padding         bool   `toml:"padding"`           // Random padding
	PaddingSize     string `toml:"padding_size"`      // "16-256" bytes
	TrafficShape    bool   `toml:"traffic_shape"`     // Traffic shaping
	SNISpoof        bool   `toml:"sni_spoof"`         // SNI spoofing
	SNISpoofDomain  string `toml:"sni_spoof_domain"`  // Fake SNI domain (e.g., "www.google.com")
	SNISpoofMethod  string `toml:"sni_spoof_method"`  // "split", "replace", "double"
	DomainFront     bool   `toml:"domain_front"`      // Domain fronting
	FrontDomain     string `toml:"front_domain"`      // CDN front domain
	RealHost        string `toml:"real_host"`         // Actual host (inside TLS)
}

// ForwardConfig configures port forwarding
type ForwardConfig struct {
	Name     string `toml:"name"`     // Friendly name
	Type     string `toml:"type"`     // "tcp", "udp", "socks5", "http"
	Listen   string `toml:"listen"`   // Local listen address
	Remote   string `toml:"remote"`   // Remote destination
}

// PathConfig configures multi-path connections
type PathConfig struct {
	Name       string `toml:"name"`
	RemoteAddr string `toml:"remote_addr"`
	Transport  string `toml:"transport"`
	Priority   int    `toml:"priority"`  // Lower = higher priority
	Weight     int    `toml:"weight"`    // For load balancing
}

// RealityConfig configures REALITY protocol
type RealityConfig struct {
	ServerName string   `toml:"server_name"` // SNI to mimic (e.g., "www.google.com")
	ShortID    string   `toml:"short_id"`
	PublicKey  string   `toml:"public_key"`
	PrivateKey string   `toml:"private_key"`
	Dest       string   `toml:"dest"`        // Fallback destination for probes
}

// KCPConfig configures KCP transport tuning
type KCPConfig struct {
	Mode       string `toml:"mode"`        // "normal", "fast", "fast2", "fast3"
	MTU        int    `toml:"mtu"`         // MTU size (default: 1350)
	SndWnd     int    `toml:"snd_wnd"`     // Send window size (default: 1024)
	RcvWnd     int    `toml:"rcv_wnd"`     // Receive window size (default: 1024)
	DataShard  int    `toml:"data_shard"`  // FEC data shards (default: 10)
	ParShard   int    `toml:"par_shard"`   // FEC parity shards (default: 3)
	SockBuf    int    `toml:"sock_buf"`    // Socket buffer size (default: 4MB)
	Encryption string `toml:"encryption"`  // "aes-128-gcm", "aes-256-gcm", "none"
}

// HealthConfig configures the health/watchdog endpoint
type HealthConfig struct {
	Enabled bool   `toml:"enabled"`
	Listen  string `toml:"listen"` // e.g. "127.0.0.1:9090"
}

// CDNModeConfig configures CDN routing
type CDNModeConfig struct {
	Enabled   bool   `toml:"enabled"`
	Provider  string `toml:"provider"`   // "cloudflare", "gcore", "arvan", "custom"
	Domain    string `toml:"domain"`     // CDN domain
	Path      string `toml:"path"`       // WebSocket path
	TLS       bool   `toml:"tls"`        // Use TLS
	EarlyData bool   `toml:"early_data"` // 0-RTT early data
}

// Load reads and parses a TOML config file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	applyDefaults(cfg)

	// Validate
	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Transport == "" {
		cfg.Transport = "tcpmux"
	}

	// Mux defaults
	if cfg.Mux.Concurrency == 0 {
		cfg.Mux.Concurrency = 8
	}
	if cfg.Mux.FrameSize == 0 {
		cfg.Mux.FrameSize = 32768 // 32KB
	}
	if cfg.Mux.RecvBuffer == 0 {
		cfg.Mux.RecvBuffer = 4194304 // 4MB
	}
	if cfg.Mux.StreamBuffer == 0 {
		cfg.Mux.StreamBuffer = 2097152 // 2MB
	}
	if cfg.Mux.MaxStreams == 0 {
		cfg.Mux.MaxStreams = 1024
	}

	// Pool defaults
	if cfg.Pool.Size == 0 {
		cfg.Pool.Size = 8
	}
	if cfg.Pool.MaxIdle == 0 {
		cfg.Pool.MaxIdle = 4
	}
	if cfg.Pool.IdleTimeout == 0 {
		cfg.Pool.IdleTimeout = 90
	}

	// Heartbeat defaults
	if cfg.Heartbeat.Interval == 0 {
		cfg.Heartbeat.Interval = 20
	}
	if cfg.Heartbeat.Timeout == 0 {
		cfg.Heartbeat.Timeout = 40
	}

	// Performance defaults
	if cfg.Performance.KeepAlive == 0 {
		cfg.Performance.KeepAlive = 15
	}
	if cfg.Performance.BufferProfile == "" {
		cfg.Performance.BufferProfile = "balanced"
	}
	if cfg.Performance.Workers == 0 {
		cfg.Performance.Workers = 4
	}

	// Anti-DPI defaults
	if cfg.AntiDPI.UTLSFingerprint == "" {
		cfg.AntiDPI.UTLSFingerprint = "chrome"
	}
	if cfg.AntiDPI.FragmentSize == "" {
		cfg.AntiDPI.FragmentSize = "50-100"
	}
	if cfg.AntiDPI.PaddingSize == "" {
		cfg.AntiDPI.PaddingSize = "16-256"
	}
}

func validate(cfg *Config) error {
	if cfg.Mode != "server" && cfg.Mode != "client" {
		return fmt.Errorf("invalid mode: %q (must be 'server' or 'client')", cfg.Mode)
	}

	if cfg.Mode == "server" && cfg.BindAddr == "" {
		return fmt.Errorf("server mode requires 'bind_addr'")
	}

	if cfg.Mode == "client" && cfg.RemoteAddr == "" && len(cfg.Paths) == 0 {
		return fmt.Errorf("client mode requires 'remote_addr' or [[paths]]")
	}

	if cfg.Password == "" {
		return fmt.Errorf("'password' is required for encryption")
	}

	validTransports := map[string]bool{
		"tcpmux": true, "wsmux": true, "reality": true,
		"h2mux": true, "shadowtls": true, "grpc": true,
		"quic": true, "kcp": true, "reverse": true,
		"faketcp": true,
	}
	if !validTransports[cfg.Transport] {
		return fmt.Errorf("invalid transport: %q", cfg.Transport)
	}

	return nil
}
