package crypto

import (
	"crypto/rand"
	"testing"
)

// BenchmarkEncrypt benchmarks AEAD encryption throughput
func BenchmarkEncrypt_1KB(b *testing.B) {
	benchEncrypt(b, 1024)
}

func BenchmarkEncrypt_4KB(b *testing.B) {
	benchEncrypt(b, 4096)
}

func BenchmarkEncrypt_16KB(b *testing.B) {
	benchEncrypt(b, 16384)
}

func BenchmarkDecrypt_1KB(b *testing.B) {
	benchDecrypt(b, 1024)
}

func BenchmarkDecrypt_4KB(b *testing.B) {
	benchDecrypt(b, 4096)
}

func BenchmarkDecrypt_16KB(b *testing.B) {
	benchDecrypt(b, 16384)
}

func benchEncrypt(b *testing.B, size int) {
	enc, err := NewEncryptor("benchmark-password-32chars-long!")
	if err != nil {
		b.Fatal(err)
	}

	data := make([]byte, size)
	rand.Read(data)

	b.SetBytes(int64(size))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := enc.Encrypt(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchDecrypt(b *testing.B, size int) {
	enc, err := NewEncryptor("benchmark-password-32chars-long!")
	if err != nil {
		b.Fatal(err)
	}

	data := make([]byte, size)
	rand.Read(data)
	ciphertext, _ := enc.Encrypt(data)

	b.SetBytes(int64(size))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := enc.Decrypt(ciphertext)
		if err != nil {
			b.Fatal(err)
		}
	}
}
