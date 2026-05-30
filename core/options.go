package core

import (
	"fmt"

	"github.com/iPmart/iPShadowT/internal/config"
)

// Option is a function that configures the engine
type Option func(*Options)

// Options holds all engine configuration options
type Options struct {
	// Config file path (if set, other options are ignored)
	ConfigFile string

	// General
	Mode      Mode
	LogLevel  string
	Transport TransportType

	// Network
	BindAddr   string
	RemoteAddr string
	Password   string

	// TLS
	TLSCert string
	TLSKey  string

	// Mux
	MuxConcurrency int
	MuxFrameSize   int

	// Anti-DPI
	AntiDPIEnabled  bool
	UTLSFingerprint string
	Fragment        bool
	FragmentSize    string
	Padding         bool
	TrafficShape    bool

	// REALITY
	RealityServerName string
	RealityPublicKey  string
	RealityPrivateKey string
	RealityShortID    string
	RealityDest       string

	// Forwards
	SOCKS5Addr string
	HTTPAddr   string
	Forwards   []ForwardOption

	// Performance
	Nodelay      bool
	KernelTuning bool
}

// ForwardOption represents a port forward configuration
type ForwardOption struct {
	Name   string
	Type   string // "tcp", "udp", "socks5", "http"
	Listen string
	Remote string
}

// --- Option functions ---

// WithConfigFile loads configuration from a TOML file
func WithConfigFile(path string) Option {
	return func(o *Options) { o.ConfigFile = path }
}

// WithMode sets the operating mode
func WithMode(mode Mode) Option {
	return func(o *Options) { o.Mode = mode }
}

// WithLogLevel sets the log level
func WithLogLevel(level string) Option {
	return func(o *Options) { o.LogLevel = level }
}

// WithTransport sets the transport type
func WithTransport(t TransportType) Option {
	return func(o *Options) { o.Transport = t }
}

// WithBindAddr sets the server bind address
func WithBindAddr(addr string) Option {
	return func(o *Options) { o.BindAddr = addr }
}

// WithRemoteAddr sets the remote server address
func WithRemoteAddr(addr string) Option {
	return func(o *Options) { o.RemoteAddr = addr }
}

// WithPassword sets the encryption password
func WithPassword(password string) Option {
	return func(o *Options) { o.Password = password }
}

// WithTLS sets TLS certificate and key paths
func WithTLS(cert, key string) Option {
	return func(o *Options) { o.TLSCert = cert; o.TLSKey = key }
}

// WithMux configures multiplexing
func WithMux(concurrency, frameSize int) Option {
	return func(o *Options) { o.MuxConcurrency = concurrency; o.MuxFrameSize = frameSize }
}

// WithAntiDPI enables anti-DPI features
func WithAntiDPI(enabled bool) Option {
	return func(o *Options) { o.AntiDPIEnabled = enabled }
}

// WithUTLS sets the uTLS fingerprint
func WithUTLS(fingerprint string) Option {
	return func(o *Options) { o.UTLSFingerprint = fingerprint; o.AntiDPIEnabled = true }
}

// WithFragment enables TLS fragmentation
func WithFragment(enabled bool, size string) Option {
	return func(o *Options) { o.Fragment = enabled; o.FragmentSize = size; o.AntiDPIEnabled = true }
}

// WithTrafficShape enables traffic shaping
func WithTrafficShape(enabled bool) Option {
	return func(o *Options) { o.TrafficShape = enabled; o.AntiDPIEnabled = true }
}

// WithReality configures REALITY protocol
func WithReality(serverName, publicKey, shortID string) Option {
	return func(o *Options) {
		o.RealityServerName = serverName
		o.RealityPublicKey = publicKey
		o.RealityShortID = shortID
		o.Transport = TransportReality
	}
}

// WithRealityServer configures REALITY for server mode
func WithRealityServer(serverName, privateKey, shortID, dest string) Option {
	return func(o *Options) {
		o.RealityServerName = serverName
		o.RealityPrivateKey = privateKey
		o.RealityShortID = shortID
		o.RealityDest = dest
		o.Transport = TransportReality
	}
}

// WithSOCKS5 adds a SOCKS5 proxy forward
func WithSOCKS5(addr string) Option {
	return func(o *Options) { o.SOCKS5Addr = addr }
}

// WithHTTPProxy adds an HTTP proxy forward
func WithHTTPProxy(addr string) Option {
	return func(o *Options) { o.HTTPAddr = addr }
}

// WithForward adds a port forward rule
func WithForward(name, fwdType, listen, remote string) Option {
	return func(o *Options) {
		o.Forwards = append(o.Forwards, ForwardOption{
			Name: name, Type: fwdType, Listen: listen, Remote: remote,
		})
	}
}

// WithNodelay enables TCP_NODELAY
func WithNodelay(enabled bool) Option {
	return func(o *Options) { o.Nodelay = enabled }
}

// WithKernelTuning enables automatic kernel tuning
func WithKernelTuning(enabled bool) Option {
	return func(o *Options) { o.KernelTuning = enabled }
}

// buildConfig converts Options to internal Config
func (o *Options) buildConfig() (*config.Config, error) {
	// If config file is specified, load from file
	if o.ConfigFile != "" {
		return config.Load(o.ConfigFile)
	}

	// Build config from options
	cfg := &config.Config{
		Mode:      string(o.Mode),
		LogLevel:  o.LogLevel,
		Transport: string(o.Transport),
		BindAddr:  o.BindAddr,
		RemoteAddr: o.RemoteAddr,
		Password:  o.Password,
		TLSCert:   o.TLSCert,
		TLSKey:    o.TLSKey,
	}

	// Defaults
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Transport == "" {
		cfg.Transport = "tcpmux"
	}
	if cfg.Mode == "" {
		return nil, fmt.Errorf("mode is required (use WithMode)")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("password is required (use WithPassword)")
	}

	// Mux
	if o.MuxConcurrency > 0 {
		cfg.Mux.Concurrency = o.MuxConcurrency
	} else {
		cfg.Mux.Concurrency = 4
	}
	if o.MuxFrameSize > 0 {
		cfg.Mux.FrameSize = o.MuxFrameSize
	} else {
		cfg.Mux.FrameSize = 32768
	}
	cfg.Mux.RecvBuffer = 4194304
	cfg.Mux.StreamBuffer = 2097152
	cfg.Mux.MaxStreams = 1024

	// Anti-DPI
	cfg.AntiDPI.Enabled = o.AntiDPIEnabled
	cfg.AntiDPI.UTLSFingerprint = o.UTLSFingerprint
	cfg.AntiDPI.Fragment = o.Fragment
	cfg.AntiDPI.FragmentSize = o.FragmentSize
	cfg.AntiDPI.Padding = o.Padding
	cfg.AntiDPI.TrafficShape = o.TrafficShape

	if cfg.AntiDPI.UTLSFingerprint == "" {
		cfg.AntiDPI.UTLSFingerprint = "chrome"
	}
	if cfg.AntiDPI.FragmentSize == "" {
		cfg.AntiDPI.FragmentSize = "50-100"
	}

	// REALITY
	cfg.Reality.ServerName = o.RealityServerName
	cfg.Reality.PublicKey = o.RealityPublicKey
	cfg.Reality.PrivateKey = o.RealityPrivateKey
	cfg.Reality.ShortID = o.RealityShortID
	cfg.Reality.Dest = o.RealityDest

	// Forwards
	if o.SOCKS5Addr != "" {
		cfg.Forwards = append(cfg.Forwards, config.ForwardConfig{
			Name: "socks5", Type: "socks5", Listen: o.SOCKS5Addr,
		})
	}
	if o.HTTPAddr != "" {
		cfg.Forwards = append(cfg.Forwards, config.ForwardConfig{
			Name: "http", Type: "http", Listen: o.HTTPAddr,
		})
	}
	for _, fwd := range o.Forwards {
		cfg.Forwards = append(cfg.Forwards, config.ForwardConfig{
			Name: fwd.Name, Type: fwd.Type, Listen: fwd.Listen, Remote: fwd.Remote,
		})
	}

	// Performance
	cfg.Performance.Nodelay = o.Nodelay
	cfg.Performance.KernelTuning = o.KernelTuning
	cfg.Performance.KeepAlive = 15
	cfg.Performance.BufferProfile = "balanced"

	// Heartbeat
	cfg.Heartbeat.Enabled = true
	cfg.Heartbeat.Interval = 20
	cfg.Heartbeat.Timeout = 40

	return cfg, nil
}
