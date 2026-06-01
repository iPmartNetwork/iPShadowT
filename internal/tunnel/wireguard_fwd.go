package tunnel

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/mux"
)

// WireGuardForwarder implements WireGuard as a forward type
// Users connect with their WireGuard client (official app, etc.)
// to this local UDP port, and traffic is tunneled through iPShadowT
//
// Flow:
//   WireGuard Client → UDP :51820 → iPShadowT tunnel → Server → Internet
//
// Config example:
//   [[forwards]]
//   name = "wireguard"
//   type = "wireguard"
//   listen = "0.0.0.0:51820"
//   remote = "10.0.0.1:51820"   # WireGuard endpoint on server side
type WireGuardForwarder struct {
	cfg      config.ForwardConfig
	pool     *mux.SessionPool
	log      *logger.Logger
	udpConn  *net.UDPConn
	done     chan struct{}
	wg       sync.WaitGroup
	clients  sync.Map // remoteAddr → *wgClient
}

// wgClient tracks a WireGuard client session
type wgClient struct {
	addr       *net.UDPAddr
	stream     *wgStreamWrapper
	lastActive time.Time
	mu         sync.Mutex
}

// wgStreamWrapper wraps a stream for the relay
type wgStreamWrapper struct {
	stream interface{ Read([]byte) (int, error); Write([]byte) (int, error); Close() error }
}

// NewWireGuardForwarder creates a new WireGuard forwarder
func NewWireGuardForwarder(cfg config.ForwardConfig, pool *mux.SessionPool, log *logger.Logger) (*WireGuardForwarder, error) {
	return &WireGuardForwarder{
		cfg:  cfg,
		pool: pool,
		log:  log,
		done: make(chan struct{}),
	}, nil
}

// Start begins listening for WireGuard UDP packets
func (wf *WireGuardForwarder) Start() error {
	addr, err := net.ResolveUDPAddr("udp", wf.cfg.Listen)
	if err != nil {
		return fmt.Errorf("resolve UDP addr: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("WireGuard forward: port %s already in use (is WireGuard running?): %w", wf.cfg.Listen, err)
	}

	wf.udpConn = conn
	wf.log.Info("[%s] WireGuard forwarder listening on %s (UDP)", wf.cfg.Name, wf.cfg.Listen)

	// Start reading UDP packets
	wf.wg.Add(1)
	go wf.readLoop()

	// Start cleanup loop for stale clients
	wf.wg.Add(1)
	go wf.cleanupLoop()

	return nil
}

// readLoop reads UDP packets from WireGuard clients
func (wf *WireGuardForwarder) readLoop() {
	defer wf.wg.Done()

	buf := make([]byte, 65535)

	for {
		select {
		case <-wf.done:
			return
		default:
		}

		wf.udpConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, remoteAddr, err := wf.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-wf.done:
				return
			default:
				wf.log.Debug("[%s] UDP read error: %v", wf.cfg.Name, err)
				continue
			}
		}

		// Get or create client session
		clientKey := remoteAddr.String()
		val, loaded := wf.clients.LoadOrStore(clientKey, &wgClient{
			addr:       remoteAddr,
			lastActive: time.Now(),
		})
		client := val.(*wgClient)
		client.lastActive = time.Now()

		if !loaded {
			// New client — establish tunnel stream
			wf.wg.Add(1)
			go wf.setupClient(client, buf[:n])
		} else {
			// Existing client — forward packet through tunnel
			wf.forwardToTunnel(client, buf[:n])
		}
	}
}

// setupClient establishes a tunnel stream for a new WireGuard client
func (wf *WireGuardForwarder) setupClient(client *wgClient, firstPacket []byte) {
	defer wf.wg.Done()

	// Get a mux stream
	stream, err := wf.pool.GetStream()
	if err != nil {
		wf.log.Error("[%s] Failed to get stream for WG client %s: %v",
			wf.cfg.Name, client.addr, err)
		wf.clients.Delete(client.addr.String())
		return
	}

	// Send destination header (WireGuard endpoint on server side)
	dest := wf.cfg.Remote
	if dest == "" {
		dest = "127.0.0.1:51820" // Default WireGuard port on server
	}
	if err := writeDestHeader(stream, dest); err != nil {
		wf.log.Error("[%s] Failed to write dest header: %v", wf.cfg.Name, err)
		stream.Close()
		wf.clients.Delete(client.addr.String())
		return
	}

	wf.log.Info("[%s] WireGuard client connected: %s → %s", wf.cfg.Name, client.addr, dest)

	// Send first packet
	wf.sendToStream(stream, firstPacket)

	// Start reading from tunnel stream → send back to WireGuard client
	wf.wg.Add(1)
	go wf.tunnelToClient(client, stream)
}

// forwardToTunnel sends a UDP packet from WireGuard client through the tunnel
func (wf *WireGuardForwarder) forwardToTunnel(client *wgClient, data []byte) {
	client.mu.Lock()
	defer client.mu.Unlock()

	if client.stream == nil {
		return // Stream not ready yet
	}

	wf.sendToStream(client.stream.stream, data)
}

// sendToStream sends a length-prefixed UDP packet over the mux stream
func (wf *WireGuardForwarder) sendToStream(stream interface{ Write([]byte) (int, error) }, data []byte) {
	// Frame: [2 bytes length][payload]
	frame := make([]byte, 2+len(data))
	frame[0] = byte(len(data) >> 8)
	frame[1] = byte(len(data))
	copy(frame[2:], data)
	stream.Write(frame)
}

// tunnelToClient reads from tunnel stream and sends UDP packets back to WireGuard client
func (wf *WireGuardForwarder) tunnelToClient(client *wgClient, stream interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}) {
	defer wf.wg.Done()
	defer stream.Close()

	// Store stream reference for sending
	client.mu.Lock()
	client.stream = &wgStreamWrapper{stream: stream}
	client.mu.Unlock()

	header := make([]byte, 2)
	for {
		select {
		case <-wf.done:
			return
		default:
		}

		// Read length header
		if _, err := readFullFromStream(stream, header); err != nil {
			wf.log.Debug("[%s] Tunnel read ended for %s: %v", wf.cfg.Name, client.addr, err)
			wf.clients.Delete(client.addr.String())
			return
		}

		pktLen := int(header[0])<<8 | int(header[1])
		if pktLen == 0 || pktLen > 65535 {
			continue
		}

		// Read payload
		pkt := make([]byte, pktLen)
		if _, err := readFullFromStream(stream, pkt); err != nil {
			wf.clients.Delete(client.addr.String())
			return
		}

		// Send back to WireGuard client via UDP
		wf.udpConn.WriteToUDP(pkt, client.addr)
	}
}

// readFullFromStream reads exactly len(buf) bytes
func readFullFromStream(stream interface{ Read([]byte) (int, error) }, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := stream.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// cleanupLoop removes stale WireGuard client sessions
func (wf *WireGuardForwarder) cleanupLoop() {
	defer wf.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wf.done:
			return
		case <-ticker.C:
			now := time.Now()
			wf.clients.Range(func(key, value interface{}) bool {
				client := value.(*wgClient)
				if now.Sub(client.lastActive) > 2*time.Minute {
					wf.log.Debug("[%s] Cleaning stale WG client: %s", wf.cfg.Name, key)
					if client.stream != nil {
						client.stream.stream.Close()
					}
					wf.clients.Delete(key)
				}
				return true
			})
		}
	}
}

// Stop stops the WireGuard forwarder
func (wf *WireGuardForwarder) Stop() {
	close(wf.done)
	if wf.udpConn != nil {
		wf.udpConn.Close()
	}
	wf.wg.Wait()
	wf.log.Info("[%s] WireGuard forwarder stopped", wf.cfg.Name)
}
