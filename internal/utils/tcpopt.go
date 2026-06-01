package utils

import (
	"crypto/tls"
	"io"
	"net"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
)

// RelayBufferSize is the copy buffer used for tunnel data relay (upload + download).
const RelayBufferSize = 256 * 1024

// TCPConnFrom unwraps TLS and other wrappers to reach the underlying *net.TCPConn.
func TCPConnFrom(conn net.Conn) *net.TCPConn {
	for conn != nil {
		if tc, ok := conn.(*net.TCPConn); ok {
			return tc
		}
		switch c := conn.(type) {
		case *tls.Conn:
			conn = c.NetConn()
		default:
			if u, ok := conn.(interface{ NetConn() net.Conn }); ok {
				conn = u.NetConn()
			} else {
				return nil
			}
		}
	}
	return nil
}

// OptimizeTCP applies socket tuning from config (port-independent).
func OptimizeTCP(conn net.Conn, perf config.PerformanceConfig) {
	tc := TCPConnFrom(conn)
	if tc == nil {
		return
	}

	if perf.Nodelay {
		_ = tc.SetNoDelay(true)
	}
	if perf.KeepAlive > 0 {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(time.Duration(perf.KeepAlive) * time.Second)
	}
	if perf.RecvBuffer > 0 {
		_ = tc.SetReadBuffer(perf.RecvBuffer)
	}
	if perf.SendBuffer > 0 {
		_ = tc.SetWriteBuffer(perf.SendBuffer)
	}
}

// CopyRelay copies data with a large buffer for better upload throughput.
func CopyRelay(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, RelayBufferSize)
	return io.CopyBuffer(dst, src, buf)
}

// OptimizedListener wraps a net.Listener and applies TCP tuning to accepted connections.
type OptimizedListener struct {
	net.Listener
	perf config.PerformanceConfig
}

// NewOptimizedListener returns a listener that tunes each accepted TCP connection.
func NewOptimizedListener(ln net.Listener, perf config.PerformanceConfig) net.Listener {
	return &OptimizedListener{Listener: ln, perf: perf}
}

func (l *OptimizedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	OptimizeTCP(conn, l.perf)
	return conn, nil
}
