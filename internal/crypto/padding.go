package crypto

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
)

// Padder adds random padding to packets to defeat traffic analysis
type Padder struct {
	minSize int
	maxSize int
	enabled bool
}

// NewPadder creates a new padder with the given size range
// sizeRange format: "min-max" (e.g., "16-256")
func NewPadder(sizeRange string, enabled bool) (*Padder, error) {
	if !enabled {
		return &Padder{enabled: false}, nil
	}

	parts := strings.Split(sizeRange, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid padding size range: %q (expected 'min-max')", sizeRange)
	}

	minSize, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid min size: %w", err)
	}

	maxSize, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("invalid max size: %w", err)
	}

	if minSize < 0 || maxSize < minSize {
		return nil, fmt.Errorf("invalid range: min=%d, max=%d", minSize, maxSize)
	}

	return &Padder{
		minSize: minSize,
		maxSize: maxSize,
		enabled: true,
	}, nil
}

// Pad adds random padding to data
// Format: [original_length (4 bytes)] [original_data] [random_padding]
func (p *Padder) Pad(data []byte) ([]byte, error) {
	if !p.enabled {
		return data, nil
	}

	// Generate random padding size
	paddingSize, err := p.randomSize()
	if err != nil {
		return nil, err
	}

	// Create padded buffer
	// Format: [4 bytes length] [data] [padding]
	result := make([]byte, 4+len(data)+paddingSize)

	// Write original data length
	binary.BigEndian.PutUint32(result[:4], uint32(len(data)))

	// Copy original data
	copy(result[4:], data)

	// Fill padding with random bytes
	if paddingSize > 0 {
		if _, err := io.ReadFull(rand.Reader, result[4+len(data):]); err != nil {
			return nil, fmt.Errorf("failed to generate padding: %w", err)
		}
	}

	return result, nil
}

// Unpad removes padding and returns original data
func (p *Padder) Unpad(data []byte) ([]byte, error) {
	if !p.enabled {
		return data, nil
	}

	if len(data) < 4 {
		return nil, fmt.Errorf("padded data too short")
	}

	// Read original length
	origLen := binary.BigEndian.Uint32(data[:4])

	if int(origLen) > len(data)-4 {
		return nil, fmt.Errorf("invalid original length: %d (available: %d)", origLen, len(data)-4)
	}

	// Extract original data (ignore padding)
	return data[4 : 4+origLen], nil
}

// randomSize generates a random padding size within the configured range
func (p *Padder) randomSize() (int, error) {
	if p.maxSize == p.minSize {
		return p.minSize, nil
	}

	rangeSize := p.maxSize - p.minSize + 1
	n, err := rand.Int(rand.Reader, big.NewInt(int64(rangeSize)))
	if err != nil {
		return 0, err
	}

	return p.minSize + int(n.Int64()), nil
}
