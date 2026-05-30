package tunnel

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/mux"
)

// Forwarder handles port forwarding for a single forward rule
type Forwarder struct {
	cfg      config.ForwardConfig
	pool     *mux.SessionPool
	log      *logger.Logger
	listener net.Listener
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewForwarder creates a new port forwarder
func NewForwarder(cfg config.ForwardConfig, pool *mux.SessionPool, log *logger.Logger) (*Forwarder, error) {
	return &Forwarder{
		cfg:  cfg,
		pool: pool,
		log:  log,
		done: make(chan struct{}),
	}, nil
}

// Start begins listening and forwarding connections
func (f *Forwarder) Start() error {
	switch f.cfg.Type {
	case "tcp":
		return f.startTCP()
	case "socks5":
		return f.startSOCKS5()
	case "http":
		return f.startHTTPProxy()
	default:
		return fmt.Errorf("unsupported forward type: %s", f.cfg.Type)
	}
}

// startTCP starts a TCP port forwarder
func (f *Forwarder) startTCP() error {
	listener, err := net.Listen("tcp", f.cfg.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", f.cfg.Listen, err)
	}

	f.listener = listener
	f.wg.Add(1)
	go f.acceptLoop()

	return nil
}

// acceptLoop accepts incoming connections
func (f *Forwarder) acceptLoop() {
	defer f.wg.Done()

	for {
		select {
		case <-f.done:
			return
		default:
		}

		conn, err := f.listener.Accept()
		if err != nil {
			select {
			case <-f.done:
				return
			default:
				f.log.Error("[%s] Accept error: %v", f.cfg.Name, err)
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		f.wg.Add(1)
		go f.handleTCPConn(conn)
	}
}

// handleTCPConn handles a single TCP connection
func (f *Forwarder) handleTCPConn(conn net.Conn) {
	defer f.wg.Done()
	defer conn.Close()

	// Get a stream from the mux pool
	stream, err := f.pool.GetStream()
	if err != nil {
		f.log.Error("[%s] Failed to get stream: %v", f.cfg.Name, err)
		return
	}
	defer stream.Close()

	// Send destination address header
	dest := f.cfg.Remote
	if err := writeDestHeader(stream, dest); err != nil {
		f.log.Error("[%s] Failed to write dest header: %v", f.cfg.Name, err)
		return
	}

	// Relay data bidirectionally
	mux.Relay(conn, stream)
}

// startSOCKS5 starts a SOCKS5 proxy server
func (f *Forwarder) startSOCKS5() error {
	listener, err := net.Listen("tcp", f.cfg.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", f.cfg.Listen, err)
	}

	f.listener = listener
	f.wg.Add(1)
	go f.socks5AcceptLoop()

	return nil
}

// socks5AcceptLoop accepts SOCKS5 connections
func (f *Forwarder) socks5AcceptLoop() {
	defer f.wg.Done()

	for {
		select {
		case <-f.done:
			return
		default:
		}

		conn, err := f.listener.Accept()
		if err != nil {
			select {
			case <-f.done:
				return
			default:
				f.log.Error("[%s] SOCKS5 accept error: %v", f.cfg.Name, err)
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		f.wg.Add(1)
		go f.handleSOCKS5(conn)
	}
}

// handleSOCKS5 handles a SOCKS5 connection
func (f *Forwarder) handleSOCKS5(conn net.Conn) {
	defer f.wg.Done()
	defer conn.Close()

	// SOCKS5 handshake
	dest, err := f.socks5Handshake(conn)
	if err != nil {
		f.log.Debug("[%s] SOCKS5 handshake failed: %v", f.cfg.Name, err)
		return
	}

	// Get a stream from the mux pool
	stream, err := f.pool.GetStream()
	if err != nil {
		f.log.Error("[%s] Failed to get stream for %s: %v", f.cfg.Name, dest, err)
		// Send SOCKS5 failure response
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer stream.Close()

	// Send destination to server
	if err := writeDestHeader(stream, dest); err != nil {
		f.log.Error("[%s] Failed to write dest: %v", f.cfg.Name, err)
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// Send SOCKS5 success response
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// Relay data
	mux.Relay(conn, stream)
}

// socks5Handshake performs the SOCKS5 protocol handshake
func (f *Forwarder) socks5Handshake(conn net.Conn) (string, error) {
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetDeadline(time.Time{})

	// Read version and auth methods
	buf := make([]byte, 258)
	n, err := conn.Read(buf)
	if err != nil || n < 2 {
		return "", fmt.Errorf("failed to read SOCKS5 greeting")
	}

	if buf[0] != 0x05 {
		return "", fmt.Errorf("not SOCKS5: version %d", buf[0])
	}

	// Reply: no auth required
	conn.Write([]byte{0x05, 0x00})

	// Read connect request
	n, err = conn.Read(buf)
	if err != nil || n < 7 {
		return "", fmt.Errorf("failed to read SOCKS5 request")
	}

	if buf[0] != 0x05 || buf[1] != 0x01 {
		return "", fmt.Errorf("unsupported SOCKS5 command: %d", buf[1])
	}

	// Parse destination address
	var host string
	var port uint16

	switch buf[3] {
	case 0x01: // IPv4
		if n < 10 {
			return "", fmt.Errorf("IPv4 address too short")
		}
		host = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		port = binary.BigEndian.Uint16(buf[8:10])

	case 0x03: // Domain
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			return "", fmt.Errorf("domain address too short")
		}
		host = string(buf[5 : 5+domainLen])
		port = binary.BigEndian.Uint16(buf[5+domainLen : 7+domainLen])

	case 0x04: // IPv6
		if n < 22 {
			return "", fmt.Errorf("IPv6 address too short")
		}
		ip := net.IP(buf[4:20])
		host = ip.String()
		port = binary.BigEndian.Uint16(buf[20:22])

	default:
		return "", fmt.Errorf("unsupported address type: %d", buf[3])
	}

	return fmt.Sprintf("%s:%d", host, port), nil
}

// startHTTPProxy starts an HTTP CONNECT proxy
func (f *Forwarder) startHTTPProxy() error {
	// HTTP proxy will be implemented in a future phase
	// For now, redirect to SOCKS5
	f.log.Warn("[%s] HTTP proxy not yet implemented, using SOCKS5", f.cfg.Name)
	return f.startSOCKS5()
}

// writeDestHeader writes the destination address header to a stream
func writeDestHeader(w io.Writer, dest string) error {
	destBytes := []byte(dest)
	header := make([]byte, 2+len(destBytes))
	header[0] = byte(len(destBytes) >> 8)
	header[1] = byte(len(destBytes))
	copy(header[2:], destBytes)

	_, err := w.Write(header)
	return err
}

// Stop gracefully stops the forwarder
func (f *Forwarder) Stop() {
	close(f.done)
	if f.listener != nil {
		f.listener.Close()
	}
	f.wg.Wait()
}
