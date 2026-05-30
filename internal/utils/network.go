package utils

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// TCPRelay copies data between two connections bidirectionally
// with optimized buffer sizes and error handling
func TCPRelay(left, right net.Conn) (int64, int64) {
	var (
		leftToRight int64
		rightToLeft int64
		wg          sync.WaitGroup
	)

	wg.Add(2)

	// left → right
	go func() {
		defer wg.Done()
		leftToRight, _ = io.Copy(right, left)
		// Signal write done
		if tc, ok := right.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// right → left
	go func() {
		defer wg.Done()
		rightToLeft, _ = io.Copy(left, right)
		if tc, ok := left.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
	return leftToRight, rightToLeft
}

// DialWithTimeout dials a TCP connection with timeout and optimizations
func DialWithTimeout(addr string, timeout time.Duration, nodelay bool) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(nodelay)
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(15 * time.Second)
	}

	return conn, nil
}

// GetLocalIP returns the primary local IP address
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "127.0.0.1", nil
}

// IsPortAvailable checks if a port is available for listening
func IsPortAvailable(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// ResolveAddr resolves a hostname to IP address
func ResolveAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid address %q: %w", addr, err)
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("DNS lookup failed for %q: %w", host, err)
	}

	// Prefer IPv4
	for _, ip := range ips {
		if ip.To4() != nil {
			return net.JoinHostPort(ip.String(), port), nil
		}
	}

	if len(ips) > 0 {
		return net.JoinHostPort(ips[0].String(), port), nil
	}

	return "", fmt.Errorf("no IP addresses found for %q", host)
}
