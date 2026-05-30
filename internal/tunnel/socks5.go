package tunnel

import (
	"fmt"
	"io"
	"net"
)

// SOCKS5 constants
const (
	SOCKS5Version = 0x05

	// Auth methods
	AuthNone     = 0x00
	AuthPassword = 0x02
	AuthNoAccept = 0xFF

	// Commands
	CmdConnect = 0x01
	CmdBind    = 0x02
	CmdUDP     = 0x03

	// Address types
	AddrIPv4   = 0x01
	AddrDomain = 0x03
	AddrIPv6   = 0x04

	// Reply codes
	RepSuccess         = 0x00
	RepServerFailure   = 0x01
	RepNotAllowed      = 0x02
	RepNetUnreachable  = 0x03
	RepHostUnreachable = 0x04
	RepConnRefused     = 0x05
	RepTTLExpired      = 0x06
	RepCmdNotSupported = 0x07
	RepAddrNotSupported = 0x08
)

// SOCKS5Reply sends a SOCKS5 reply to the client
func SOCKS5Reply(w io.Writer, rep byte, addr net.Addr) error {
	reply := []byte{SOCKS5Version, rep, 0x00, AddrIPv4, 0, 0, 0, 0, 0, 0}

	if addr != nil {
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			ip := tcpAddr.IP.To4()
			if ip != nil {
				copy(reply[4:8], ip)
				reply[8] = byte(tcpAddr.Port >> 8)
				reply[9] = byte(tcpAddr.Port)
			}
		}
	}

	_, err := w.Write(reply)
	return err
}

// ParseSOCKS5Addr parses a SOCKS5 address from a reader
func ParseSOCKS5Addr(r io.Reader) (string, error) {
	// Read address type
	addrType := make([]byte, 1)
	if _, err := io.ReadFull(r, addrType); err != nil {
		return "", err
	}

	var host string
	switch addrType[0] {
	case AddrIPv4:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(r, ip); err != nil {
			return "", err
		}
		host = net.IP(ip).String()

	case AddrDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(r, lenBuf); err != nil {
			return "", err
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(r, domain); err != nil {
			return "", err
		}
		host = string(domain)

	case AddrIPv6:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(r, ip); err != nil {
			return "", err
		}
		host = net.IP(ip).String()

	default:
		return "", fmt.Errorf("unsupported address type: %d", addrType[0])
	}

	// Read port
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, portBuf); err != nil {
		return "", err
	}
	port := int(portBuf[0])<<8 | int(portBuf[1])

	return fmt.Sprintf("%s:%d", host, port), nil
}
