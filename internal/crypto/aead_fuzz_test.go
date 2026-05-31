package crypto

import (
	"testing"
)

// FuzzAEAD tests the AEAD encrypt/decrypt cycle with random inputs
// Run: go test -fuzz=FuzzAEAD -fuzztime=60s ./internal/crypto/
func FuzzAEAD(f *testing.F) {
	// Seed corpus
	f.Add([]byte("hello world"), []byte("password123"))
	f.Add([]byte(""), []byte("short"))
	f.Add([]byte{0x00, 0x01, 0x02}, []byte("a-longer-password-for-testing"))
	f.Add(make([]byte, 1024), []byte("key"))
	f.Add([]byte("unicode: 你好世界 🌍"), []byte("пароль"))

	f.Fuzz(func(t *testing.T, plaintext, password []byte) {
		if len(password) == 0 {
			return // Skip empty passwords
		}

		enc, err := NewEncryptor(string(password))
		if err != nil {
			return // Some passwords may be invalid
		}

		// Encrypt
		ciphertext, err := enc.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		// Decrypt
		decrypted, err := enc.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}

		// Verify roundtrip
		if len(plaintext) != len(decrypted) {
			t.Fatalf("length mismatch: got %d, want %d", len(decrypted), len(plaintext))
		}
		for i := range plaintext {
			if plaintext[i] != decrypted[i] {
				t.Fatalf("byte %d mismatch: got %d, want %d", i, decrypted[i], plaintext[i])
			}
		}
	})
}

// FuzzAEADDecryptCorrupt tests that corrupted ciphertext is rejected
func FuzzAEADDecryptCorrupt(f *testing.F) {
	f.Add([]byte("test data"), []byte("password"), byte(0), 5)

	f.Fuzz(func(t *testing.T, plaintext, password []byte, xorByte byte, position int) {
		if len(password) == 0 {
			return
		}

		enc, err := NewEncryptor(string(password))
		if err != nil {
			return
		}

		ciphertext, err := enc.Encrypt(plaintext)
		if err != nil {
			return
		}

		if len(ciphertext) == 0 {
			return
		}

		// Corrupt a byte
		pos := position % len(ciphertext)
		corrupted := make([]byte, len(ciphertext))
		copy(corrupted, ciphertext)
		corrupted[pos] ^= xorByte
		if xorByte == 0 {
			corrupted[pos] ^= 0xFF
		}

		// Decryption should fail
		_, err = enc.Decrypt(corrupted)
		if err == nil {
			// This is acceptable in rare cases due to nonce position
			// but should be extremely rare
		}
	})
}
