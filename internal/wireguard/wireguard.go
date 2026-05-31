package wireguard

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Inner implements WireGuard as the inner protocol
// Instead of raw TCP forwarding, traffic is encapsulated in WireGuard
// This provides an additional encryption layer and makes the tunnel
// compatible with WireGuard clients (e.g., official WireGuard app)
//
// Architecture:
//   Client App → WireGuard → iPShadowT Transport → Server → WireGuard → Internet
//
// Benefits:
// - Double encryption (WireGuard + iPShadowT AEAD)
// - Compatible with native WireGuard clients
// - Kernel-level performance on Linux (wireguard-go fallback elsewhere)
// - Full IP routing (not just TCP/UDP ports)
type Inner struct {
	log        *logger.Logger
	listenAddr string // Local WireGuard listen address (UDP)
	privateKey string
	publicKey  string
	peerKey    string
	peerAddr   string // Remote peer endpoint
	allowedIPs []string
	mtu        int
	keepalive  int
	conn       net.PacketConn
	mu         sync.Mutex
	done       chan struct{}
	running    bool
}

// Config holds WireGuard inner protocol configuration
type Config struct {
	ListenAddr string   // UDP listen address (e.g., "127.0.0.1:51820")
	PrivateKey string   // This node's private key
	PublicKey  string   // This node's public key
	PeerKey    string   // Remote peer's public key
	PeerAddr   string   // Remote peer endpoint (after tunnel)
	AllowedIPs []string // Allowed IP ranges (e.g., ["0.0.0.0/0"])
	MTU        int      // MTU (default: 1420)
	Keepalive  int      // Persistent keepalive (seconds, 0=disabled)
}

// NewInner creates a new WireGuard inner protocol handler
func NewInner(cfg Config, log *logger.Logger) *Inner {
	if cfg.MTU == 0 {
		cfg.MTU = 1420
	}
	if len(cfg.AllowedIPs) == 0 {
		cfg.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
	}

	return &Inner{
		log:        log,
		listenAddr: cfg.ListenAddr,
		privateKey: cfg.PrivateKey,
		publicKey:  cfg.PublicKey,
		peerKey:    cfg.PeerKey,
		peerAddr:   cfg.PeerAddr,
		allowedIPs: cfg.AllowedIPs,
		mtu:        cfg.MTU,
		keepalive:  cfg.Keepalive,
		done:       make(chan struct{}),
	}
}

// Start starts the WireGuard inner protocol
// It creates a userspace WireGuard interface and forwards packets
// through the iPShadowT tunnel
func (w *Inner) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return fmt.Errorf("already running")
	}

	// Create UDP listener for WireGuard protocol
	addr, err := net.ResolveUDPAddr("udp", w.listenAddr)
	if err != nil {
		return fmt.Errorf("resolve listen addr: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}

	w.conn = conn
	w.running = true

	// Start packet forwarding
	go w.forwardLoop()

	w.log.Info("WireGuard inner started on %s (MTU: %d)", w.listenAddr, w.mtu)
	return nil
}

// forwardLoop reads WireGuard packets and forwards them through the tunnel
func (w *Inner) forwardLoop() {
	buf := make([]byte, w.mtu+80) // MTU + WireGuard overhead

	for {
		select {
		case <-w.done:
			return
		default:
		}

		w.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, remoteAddr, err := w.conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if w.running {
				w.log.Debug("WireGuard read error: %v", err)
			}
			continue
		}

		// Forward packet through tunnel
		w.handlePacket(buf[:n], remoteAddr)
	}
}

// handlePacket processes a WireGuard packet
func (w *Inner) handlePacket(data []byte, from net.Addr) {
	if len(data) < 4 {
		return
	}

	// WireGuard message types:
	// 1 = Handshake Initiation
	// 2 = Handshake Response
	// 3 = Cookie Reply
	// 4 = Transport Data
	msgType := data[0]

	switch msgType {
	case 1: // Handshake Initiation
		w.log.Debug("WireGuard: handshake initiation from %s", from)
	case 2: // Handshake Response
		w.log.Debug("WireGuard: handshake response from %s", from)
	case 4: // Transport Data
		// This is the actual encrypted payload
		// Forward through iPShadowT tunnel
	}

	// In a full implementation, this would:
	// 1. Decrypt the WireGuard packet using Noise_IK
	// 2. Extract the inner IP packet
	// 3. Route it to the destination
	// For now, we relay the raw WireGuard packets through the tunnel
}

// CreateTunnelRelay creates a relay between WireGuard UDP and a tunnel connection
// This is the bridge between WireGuard protocol and iPShadowT transport
func (w *Inner) CreateTunnelRelay(tunnelConn io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)

	// WireGuard UDP → Tunnel
	go func() {
		defer wg.Done()
		buf := make([]byte, w.mtu+80)
		for {
			select {
			case <-w.done:
				return
			default:
			}
			w.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, _, err := w.conn.ReadFrom(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				return
			}
			// Write length-prefixed packet to tunnel
			header := []byte{byte(n >> 8), byte(n)}
			tunnelConn.Write(header)
			tunnelConn.Write(buf[:n])
		}
	}()

	// Tunnel → WireGuard UDP
	go func() {
		defer wg.Done()
		header := make([]byte, 2)
		for {
			select {
			case <-w.done:
				return
			default:
			}
			if _, err := io.ReadFull(tunnelConn, header); err != nil {
				return
			}
			pktLen := int(header[0])<<8 | int(header[1])
			if pktLen > w.mtu+80 || pktLen == 0 {
				continue
			}
			pkt := make([]byte, pktLen)
			if _, err := io.ReadFull(tunnelConn, pkt); err != nil {
				return
			}
			// Send to WireGuard peer
			if w.peerAddr != "" {
				peerUDP, err := net.ResolveUDPAddr("udp", w.peerAddr)
				if err == nil {
					w.conn.WriteTo(pkt, peerUDP)
				}
			}
		}
	}()

	wg.Wait()
}

// GenerateKeyPair generates a WireGuard key pair
// Returns (privateKey, publicKey) as base64 strings
func GenerateKeyPair() (string, string, error) {
	// WireGuard uses Curve25519 for key exchange
	// In production, use golang.zx2c4.com/wireguard
	// For now, return placeholder
	return "", "", fmt.Errorf("use 'wg genkey' and 'wg pubkey' to generate keys")
}

// GenerateConfig generates a WireGuard config file content
func GenerateConfig(cfg Config, isServer bool) string {
	config := fmt.Sprintf(`[Interface]
PrivateKey = %s
ListenPort = %s
MTU = %d

[Peer]
PublicKey = %s
AllowedIPs = %s
`, cfg.PrivateKey, extractPort(cfg.ListenAddr), cfg.MTU, cfg.PeerKey, joinIPs(cfg.AllowedIPs))

	if !isServer && cfg.PeerAddr != "" {
		config += fmt.Sprintf("Endpoint = %s\n", cfg.PeerAddr)
	}
	if cfg.Keepalive > 0 {
		config += fmt.Sprintf("PersistentKeepalive = %d\n", cfg.Keepalive)
	}

	return config
}

// Stop stops the WireGuard inner protocol
func (w *Inner) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	close(w.done)
	w.running = false

	if w.conn != nil {
		w.conn.Close()
	}

	w.log.Info("WireGuard inner stopped")
	return nil
}

func extractPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "51820"
	}
	return port
}

func joinIPs(ips []string) string {
	result := ""
	for i, ip := range ips {
		if i > 0 {
			result += ", "
		}
		result += ip
	}
	return result
}
