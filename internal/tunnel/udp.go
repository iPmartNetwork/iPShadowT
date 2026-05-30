package tunnel

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/mux"
)

// UDPForwarder handles UDP port forwarding over the tunnel
// UDP packets are encapsulated and sent over the mux stream
type UDPForwarder struct {
	listenAddr string
	remoteAddr string
	pool       *mux.SessionPool
	log        *logger.Logger
	conn       *net.UDPConn
	sessions   sync.Map // map[string]*udpSession
	done       chan struct{}
}

type udpSession struct {
	clientAddr *net.UDPAddr
	lastActive time.Time
	stream     interface{} // mux stream
}

// NewUDPForwarder creates a new UDP forwarder
func NewUDPForwarder(listen, remote string, pool *mux.SessionPool, log *logger.Logger) *UDPForwarder {
	return &UDPForwarder{
		listenAddr: listen,
		remoteAddr: remote,
		pool:       pool,
		log:        log,
		done:       make(chan struct{}),
	}
}

// Start begins UDP forwarding
func (u *UDPForwarder) Start() error {
	addr, err := net.ResolveUDPAddr("udp", u.listenAddr)
	if err != nil {
		return fmt.Errorf("resolve UDP addr failed: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("UDP listen failed: %w", err)
	}

	u.conn = conn
	go u.readLoop()
	go u.cleanupLoop()

	u.log.Info("UDP forwarder: %s → %s", u.listenAddr, u.remoteAddr)
	return nil
}

// readLoop reads UDP packets and forwards them through the tunnel
func (u *UDPForwarder) readLoop() {
	buf := make([]byte, 65535)

	for {
		select {
		case <-u.done:
			return
		default:
		}

		u.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, clientAddr, err := u.conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		// Get or create session for this client
		key := clientAddr.String()
		_, loaded := u.sessions.LoadOrStore(key, &udpSession{
			clientAddr: clientAddr,
			lastActive: time.Now(),
		})

		if !loaded {
			u.log.Debug("UDP: new session from %s", key)
		}

		// Update last active
		if val, ok := u.sessions.Load(key); ok {
			val.(*udpSession).lastActive = time.Now()
		}

		// Forward through tunnel
		go u.forwardPacket(clientAddr, buf[:n])
	}
}

// forwardPacket sends a UDP packet through the mux tunnel
func (u *UDPForwarder) forwardPacket(clientAddr *net.UDPAddr, data []byte) {
	stream, err := u.pool.GetStream()
	if err != nil {
		u.log.Debug("UDP: failed to get stream: %v", err)
		return
	}
	defer stream.Close()

	// Write destination header
	dest := []byte(u.remoteAddr)
	header := make([]byte, 4+len(dest)+len(data))
	// [type(1)=UDP] [dest_len(1)] [dest] [data_len(2)] [data]
	header[0] = 0x02 // UDP type marker
	header[1] = byte(len(dest))
	copy(header[2:], dest)
	header[2+len(dest)] = byte(len(data) >> 8)
	header[3+len(dest)] = byte(len(data))
	copy(header[4+len(dest):], data)

	stream.Write(header)

	// Read response
	respBuf := make([]byte, 65535)
	stream.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := stream.Read(respBuf)
	if err != nil {
		return
	}

	// Send response back to client
	u.conn.WriteToUDP(respBuf[:n], clientAddr)
}

// cleanupLoop removes inactive sessions
func (u *UDPForwarder) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-u.done:
			return
		case <-ticker.C:
			u.sessions.Range(func(key, value interface{}) bool {
				session := value.(*udpSession)
				if time.Since(session.lastActive) > 2*time.Minute {
					u.sessions.Delete(key)
				}
				return true
			})
		}
	}
}

// Stop stops the UDP forwarder
func (u *UDPForwarder) Stop() {
	close(u.done)
	if u.conn != nil {
		u.conn.Close()
	}
}
