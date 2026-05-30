package transport

import (
	"io"
	"net"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

// Transport defines the interface for all transport implementations
type Transport interface {
	// Name returns the transport name
	Name() string

	// Dial creates a new connection to the remote server (client-side)
	Dial() (net.Conn, error)

	// Listen starts accepting connections (server-side)
	Listen() (net.Listener, error)

	// Close shuts down the transport
	Close() error
}

// Stream represents a multiplexed stream over a transport connection
type Stream interface {
	io.ReadWriteCloser
	// LocalAddr returns the local address
	LocalAddr() net.Addr
	// RemoteAddr returns the remote address
	RemoteAddr() net.Addr
}

// NewTransport creates a transport based on config
func NewTransport(cfg *config.Config, log *logger.Logger) (Transport, error) {
	switch cfg.Transport {
	case "tcpmux":
		return NewTCPMux(cfg, log), nil
	case "wsmux":
		return NewWSMux(cfg, log), nil
	case "reality":
		return NewReality(cfg, log)
	case "h2mux":
		return NewH2Mux(cfg, log), nil
	case "shadowtls":
		return NewShadowTLS(cfg, log), nil
	case "grpc":
		return NewGRPC(cfg, log), nil
	case "quic":
		return NewQUIC(cfg, log), nil
	case "kcp":
		return NewKCP(cfg, log), nil
	case "reverse":
		return NewReverse(cfg, log)
	default:
		return NewTCPMux(cfg, log), nil
	}
}
