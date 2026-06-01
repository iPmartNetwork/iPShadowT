package tunnel

import (
	"fmt"
	"io"
)

const MaxDestLen = 512

// WriteDestHeader writes the destination address header to a stream.
func WriteDestHeader(w io.Writer, dest string) error {
	destBytes := []byte(dest)
	if len(destBytes) > MaxDestLen {
		return fmt.Errorf("destination too long: %d bytes", len(destBytes))
	}
	header := make([]byte, 2+len(destBytes))
	header[0] = byte(len(destBytes) >> 8)
	header[1] = byte(len(destBytes))
	copy(header[2:], destBytes)
	_, err := w.Write(header)
	return err
}

// ReadDestHeader reads the destination address from a stream header.
func ReadDestHeader(r io.Reader) (string, error) {
	destBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, destBuf); err != nil {
		return "", err
	}
	destLen := int(destBuf[0])<<8 | int(destBuf[1])
	if destLen <= 0 || destLen > MaxDestLen {
		return "", fmt.Errorf("invalid destination length: %d", destLen)
	}
	destAddr := make([]byte, destLen)
	if _, err := io.ReadFull(r, destAddr); err != nil {
		return "", err
	}
	return string(destAddr), nil
}
