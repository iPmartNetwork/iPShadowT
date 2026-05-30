package antidpi

import (
	"fmt"
	"net"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// Engine is the main anti-DPI engine that combines all techniques
type Engine struct {
	cfg         *config.AntiDPIConfig
	log         *logger.Logger
	utls        *UTLSDialer
	fragmenter  *Fragmenter
	shaper      *TrafficShaper
	probeResist *ProbeResistance
	enabled     bool
}

// NewEngine creates a new anti-DPI engine
func NewEngine(cfg *config.AntiDPIConfig, realityCfg *config.RealityConfig, log *logger.Logger) (*Engine, error) {
	if cfg == nil || !cfg.Enabled {
		return &Engine{enabled: false, log: log}, nil
	}

	engine := &Engine{
		cfg:     cfg,
		log:     log,
		enabled: true,
	}

	// Initialize uTLS
	serverName := ""
	if realityCfg != nil {
		serverName = realityCfg.ServerName
	}
	engine.utls = NewUTLSDialer(cfg.UTLSFingerprint, serverName, log)

	// Initialize fragmenter
	fragmenter, err := NewFragmenter(cfg.FragmentSize, cfg.Fragment, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create fragmenter: %w", err)
	}
	engine.fragmenter = fragmenter

	// Initialize traffic shaper
	engine.shaper = NewTrafficShaper(cfg.TrafficShape, log)

	log.Info("Anti-DPI engine initialized:")
	log.Info("  uTLS Fingerprint: %s", cfg.UTLSFingerprint)
	log.Info("  TLS Fragmentation: %v", cfg.Fragment)
	log.Info("  Random Padding: %v", cfg.Padding)
	log.Info("  Traffic Shaping: %v", cfg.TrafficShape)

	return engine, nil
}

// WrapClientConn applies all anti-DPI techniques to a client connection
// Call order:
// 1. Fragment (splits TLS ClientHello)
// 2. uTLS (mimics browser fingerprint)
// 3. Traffic Shaping (normalizes traffic patterns)
func (e *Engine) WrapClientConn(conn net.Conn, sni string) (net.Conn, error) {
	if !e.enabled {
		return conn, nil
	}

	var wrappedConn net.Conn = conn

	// Step 1: Apply TLS fragmentation (wraps the connection)
	if e.fragmenter != nil && e.cfg.Fragment {
		wrappedConn = NewFragmentedConn(wrappedConn, e.fragmenter)
		e.log.Debug("Applied TLS fragmentation")
	}

	// Step 2: Apply uTLS (performs TLS handshake with browser fingerprint)
	if e.utls != nil && sni != "" {
		tlsConn, err := e.utls.WrapConn(wrappedConn, sni)
		if err != nil {
			return nil, fmt.Errorf("uTLS wrap failed: %w", err)
		}
		wrappedConn = tlsConn
		e.log.Debug("Applied uTLS fingerprint: %s", e.cfg.UTLSFingerprint)
	}

	// Step 3: Apply traffic shaping
	if e.shaper != nil && e.cfg.TrafficShape {
		wrappedConn = e.shaper.WrapConn(wrappedConn)
		e.log.Debug("Applied traffic shaping")
	}

	return wrappedConn, nil
}

// SetupProbeResistance initializes probe resistance for the server
func (e *Engine) SetupProbeResistance(fallbackDest string) {
	e.probeResist = NewProbeResistance(fallbackDest, e.log)
	e.log.Info("Probe resistance enabled (fallback: %s)", fallbackDest)
}

// HandleProbe checks if a connection is a probe and handles it
func (e *Engine) HandleProbe(conn net.Conn, expectedToken []byte) (bool, error) {
	if e.probeResist == nil {
		return true, nil // No probe resistance, accept all
	}
	return e.probeResist.HandleConnection(conn, expectedToken)
}

// GetFragmenter returns the fragmenter instance
func (e *Engine) GetFragmenter() *Fragmenter {
	return e.fragmenter
}

// GetShaper returns the traffic shaper instance
func (e *Engine) GetShaper() *TrafficShaper {
	return e.shaper
}

// IsEnabled returns whether anti-DPI is enabled
func (e *Engine) IsEnabled() bool {
	return e.enabled
}
