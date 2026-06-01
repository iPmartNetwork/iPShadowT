package transport

import (
	"fmt"
	"net"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// Pipeline implements a chain-of-transports architecture
//
// Inspired by: radkesvat/WaterWall — pipeline/node-based processing
//
// How it works:
// 1. Multiple transport layers are chained together
// 2. Each layer wraps the connection from the previous layer
// 3. Data flows through all layers in order
//
// Example chains:
// - faketcp → tcpmux (UDP apps over fake TCP with mux)
// - tls → wsmux (TLS encrypted WebSocket mux)
// - faketcp → tls → h2mux (fake TCP carrying TLS HTTP/2)
//
// This allows combining transports for maximum stealth:
// DPI sees: TCP connection (layer 1)
// Inside: TLS with whitelisted SNI (layer 2)
// Inside: HTTP/2 multiplexed streams (layer 3)
// Inside: Encrypted tunnel data (layer 4)
type Pipeline struct {
	layers []PipelineLayer
	cfg    *config.Config
	log    *logger.Logger
}

// PipelineLayer represents a single layer in the transport pipeline
type PipelineLayer struct {
	Name      string
	Transport Transport
	WrapConn  func(net.Conn) (net.Conn, error) // Optional connection wrapper
}

// PipelineConfig configures the transport pipeline
type PipelineConfig struct {
	// Layers defines the transport chain (outer to inner)
	// Example: ["faketcp", "tls", "h2mux"]
	Layers []string
}

// NewPipeline creates a new transport pipeline
func NewPipeline(cfg *config.Config, pipelineCfg PipelineConfig, log *logger.Logger) (*Pipeline, error) {
	if len(pipelineCfg.Layers) == 0 {
		return nil, fmt.Errorf("pipeline requires at least one layer")
	}

	p := &Pipeline{
		layers: make([]PipelineLayer, 0, len(pipelineCfg.Layers)),
		cfg:    cfg,
		log:    log,
	}

	// Build layers
	for _, layerName := range pipelineCfg.Layers {
		layer := PipelineLayer{Name: layerName}

		switch layerName {
		case "faketcp":
			layer.Transport = NewFakeTCP(cfg, log)
		case "tcpmux":
			layer.Transport = NewTCPMux(cfg, log)
		case "wsmux":
			layer.Transport = NewWSMux(cfg, log)
		case "h2mux":
			layer.Transport = NewH2Mux(cfg, log)
		case "quic":
			layer.Transport = NewQUIC(cfg, log)
		default:
			return nil, fmt.Errorf("unknown pipeline layer: %s", layerName)
		}

		p.layers = append(p.layers, layer)
	}

	return p, nil
}

// Name returns the pipeline name
func (p *Pipeline) Name() string {
	name := "pipeline["
	for i, l := range p.layers {
		if i > 0 {
			name += "→"
		}
		name += l.Name
	}
	name += "]"
	return name
}

// Dial connects through all pipeline layers (client-side)
// Connects outer layer first, then wraps with inner layers
func (p *Pipeline) Dial() (net.Conn, error) {
	if len(p.layers) == 0 {
		return nil, fmt.Errorf("empty pipeline")
	}

	// First layer: actual network dial
	conn, err := p.layers[0].Transport.Dial()
	if err != nil {
		return nil, fmt.Errorf("pipeline layer %s dial failed: %w", p.layers[0].Name, err)
	}

	p.log.Debug("Pipeline: connected layer 0 (%s)", p.layers[0].Name)

	// Subsequent layers: wrap the connection
	for i := 1; i < len(p.layers); i++ {
		layer := p.layers[i]
		if layer.WrapConn != nil {
			conn, err = layer.WrapConn(conn)
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("pipeline layer %s wrap failed: %w", layer.Name, err)
			}
			p.log.Debug("Pipeline: wrapped layer %d (%s)", i, layer.Name)
		}
	}

	return conn, nil
}

// Listen starts the pipeline listener (server-side)
// Only the outermost layer listens; inner layers unwrap
func (p *Pipeline) Listen() (net.Listener, error) {
	if len(p.layers) == 0 {
		return nil, fmt.Errorf("empty pipeline")
	}

	// Outer layer listens
	ln, err := p.layers[0].Transport.Listen()
	if err != nil {
		return nil, fmt.Errorf("pipeline listen failed: %w", err)
	}

	p.log.Info("Pipeline listening: %s", p.Name())
	return &pipelineListener{
		inner:  ln,
		layers: p.layers[1:],
		log:    p.log,
	}, nil
}

// Close shuts down all layers
func (p *Pipeline) Close() error {
	for _, l := range p.layers {
		if l.Transport != nil {
			l.Transport.Close()
		}
	}
	return nil
}

// pipelineListener wraps accepted connections through remaining layers
type pipelineListener struct {
	inner  net.Listener
	layers []PipelineLayer
	log    *logger.Logger
}

func (l *pipelineListener) Accept() (net.Conn, error) {
	conn, err := l.inner.Accept()
	if err != nil {
		return nil, err
	}

	// Unwrap through remaining layers
	for i, layer := range l.layers {
		if layer.WrapConn != nil {
			conn, err = layer.WrapConn(conn)
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("pipeline accept layer %d (%s) failed: %w", i, layer.Name, err)
			}
		}
	}

	return conn, nil
}

func (l *pipelineListener) Close() error { return l.inner.Close() }
func (l *pipelineListener) Addr() net.Addr { return l.inner.Addr() }

// CommonPipelines returns pre-defined pipeline configurations
func CommonPipelines() map[string]PipelineConfig {
	return map[string]PipelineConfig{
		// UDP apps (WireGuard/QUIC) over fake TCP
		"udp-over-tcp": {Layers: []string{"faketcp"}},
		// Maximum stealth: fake TCP → TLS → HTTP/2
		"max-stealth": {Layers: []string{"faketcp", "h2mux"}},
		// CDN compatible: WebSocket with mux
		"cdn-ready": {Layers: []string{"wsmux"}},
	}
}
