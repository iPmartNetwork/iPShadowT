package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	enc, err := NewEncryptor("test-password-123")
	if err != nil {
		t.Fatalf("NewEncryptor failed: %v", err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("hello")},
		{"medium", []byte("this is a medium length test message for encryption")},
		{"binary", []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}},
		{"large", bytes.Repeat([]byte("A"), 10000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			ciphertext, err := enc.Encrypt(tt.data)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Ciphertext should be different from plaintext
			if len(tt.data) > 0 && bytes.Equal(ciphertext, tt.data) {
				t.Fatal("Ciphertext equals plaintext")
			}

			// Decrypt
			plaintext, err := enc.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			// Should match original
			if !bytes.Equal(plaintext, tt.data) {
				t.Fatalf("Decrypted data doesn't match: got %v, want %v", plaintext, tt.data)
			}
		})
	}
}

func TestEncryptDecryptWrongKey(t *testing.T) {
	enc1, _ := NewEncryptor("password-1")
	enc2, _ := NewEncryptor("password-2")

	data := []byte("secret message")

	ciphertext, err := enc1.Encrypt(data)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypting with wrong key should fail
	_, err = enc2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("Expected error when decrypting with wrong key")
	}
}

func TestEncryptDifferentNonce(t *testing.T) {
	enc, _ := NewEncryptor("test-password")
	data := []byte("same data")

	ct1, _ := enc.Encrypt(data)
	ct2, _ := enc.Encrypt(data)

	// Same plaintext should produce different ciphertext (random nonce)
	if bytes.Equal(ct1, ct2) {
		t.Fatal("Two encryptions of same data produced same ciphertext")
	}
}

func TestMaxPayloadSize(t *testing.T) {
	enc, _ := NewEncryptor("test")

	// Should succeed at max size
	data := make([]byte, MaxPayloadSize)
	_, err := enc.Encrypt(data)
	if err != nil {
		t.Fatalf("Encrypt at max size failed: %v", err)
	}

	// Should fail above max size
	data = make([]byte, MaxPayloadSize+1)
	_, err = enc.Encrypt(data)
	if err == nil {
		t.Fatal("Expected error for oversized payload")
	}
}

func BenchmarkEncrypt(b *testing.B) {
	enc, _ := NewEncryptor("benchmark-password")
	data := make([]byte, 4096)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.Encrypt(data)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	enc, _ := NewEncryptor("benchmark-password")
	data := make([]byte, 4096)
	ciphertext, _ := enc.Encrypt(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.Decrypt(ciphertext)
	}
}
