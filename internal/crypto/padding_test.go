package crypto

import (
	"bytes"
	"testing"
)

func TestPadUnpad(t *testing.T) {
	padder, err := NewPadder("16-256", true)
	if err != nil {
		t.Fatalf("NewPadder failed: %v", err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("hi")},
		{"medium", []byte("hello world, this is a test")},
		{"binary", []byte{0x00, 0xFF, 0x80}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			padded, err := padder.Pad(tt.data)
			if err != nil {
				t.Fatalf("Pad failed: %v", err)
			}

			// Padded should be larger
			if len(padded) <= len(tt.data) {
				t.Fatal("Padded data not larger than original")
			}

			// Unpad
			unpadded, err := padder.Unpad(padded)
			if err != nil {
				t.Fatalf("Unpad failed: %v", err)
			}

			if !bytes.Equal(unpadded, tt.data) {
				t.Fatalf("Unpadded doesn't match: got %v, want %v", unpadded, tt.data)
			}
		})
	}
}

func TestPadderDisabled(t *testing.T) {
	padder, _ := NewPadder("16-256", false)
	data := []byte("test data")

	padded, err := padder.Pad(data)
	if err != nil {
		t.Fatalf("Pad failed: %v", err)
	}

	// When disabled, should return original data
	if !bytes.Equal(padded, data) {
		t.Fatal("Disabled padder should return original data")
	}
}

func TestPadderInvalidRange(t *testing.T) {
	_, err := NewPadder("invalid", true)
	if err == nil {
		t.Fatal("Expected error for invalid range")
	}

	_, err = NewPadder("100-50", true)
	if err == nil {
		t.Fatal("Expected error for reversed range")
	}
}
