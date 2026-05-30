package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// MaxPayloadSize is the maximum size of a single encrypted payload
	MaxPayloadSize = 65535
	// NonceSize is the size of the nonce for ChaCha20-Poly1305
	NonceSize = chacha20poly1305.NonceSizeX // 24 bytes for XChaCha20
	// TagSize is the authentication tag size
	TagSize = chacha20poly1305.Overhead // 16 bytes
	// LengthSize is the size of the length prefix
	LengthSize = 2
	// HeaderSize is nonce + length + tag overhead
	HeaderSize = NonceSize + LengthSize + TagSize
)

// Encryptor handles AEAD encryption/decryption
type Encryptor struct {
	aead    cipher.AEAD
	nonce   uint64
	nonceMu sync.Mutex
}

// NewEncryptor creates a new encryptor from a password
func NewEncryptor(password string) (*Encryptor, error) {
	// Derive key from password using SHA-256
	key := deriveKey(password)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	return &Encryptor{
		aead: aead,
	}, nil
}

// deriveKey derives a 32-byte key from password using SHA-256
func deriveKey(password string) []byte {
	// Use SHA-256 for key derivation (simple but effective)
	// For production, consider Argon2 or HKDF
	hash := sha256.Sum256([]byte(password))
	return hash[:]
}

// Encrypt encrypts plaintext with random nonce and returns ciphertext
// Format: [nonce (24)] [encrypted_length (2+16)] [encrypted_payload (N+16)]
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) > MaxPayloadSize {
		return nil, errors.New("payload too large")
	}

	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encode length
	lengthBuf := make([]byte, LengthSize)
	binary.BigEndian.PutUint16(lengthBuf, uint16(len(plaintext)))

	// Encrypt length (to hide payload size from DPI)
	encryptedLength := e.aead.Seal(nil, nonce, lengthBuf, nil)

	// Encrypt payload
	encryptedPayload := e.aead.Seal(nil, nonce, plaintext, nil)

	// Combine: nonce + encrypted_length + encrypted_payload
	result := make([]byte, 0, NonceSize+len(encryptedLength)+len(encryptedPayload))
	result = append(result, nonce...)
	result = append(result, encryptedLength...)
	result = append(result, encryptedPayload...)

	return result, nil
}

// Decrypt decrypts ciphertext
func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < NonceSize+LengthSize+TagSize {
		return nil, errors.New("ciphertext too short")
	}

	// Extract nonce
	nonce := ciphertext[:NonceSize]

	// Decrypt length
	encryptedLength := ciphertext[NonceSize : NonceSize+LengthSize+TagSize]
	lengthBuf, err := e.aead.Open(nil, nonce, encryptedLength, nil)
	if err != nil {
		return nil, errors.New("failed to decrypt length: invalid key or corrupted data")
	}

	payloadLen := binary.BigEndian.Uint16(lengthBuf)

	// Decrypt payload
	encryptedPayload := ciphertext[NonceSize+LengthSize+TagSize:]
	if len(encryptedPayload) < int(payloadLen)+TagSize {
		return nil, errors.New("payload too short")
	}

	plaintext, err := e.aead.Open(nil, nonce, encryptedPayload[:payloadLen+TagSize], nil)
	if err != nil {
		return nil, errors.New("failed to decrypt payload: invalid key or corrupted data")
	}

	return plaintext, nil
}

// EncryptedReader wraps an io.Reader with decryption
type EncryptedReader struct {
	reader    io.Reader
	encryptor *Encryptor
	buf       []byte
}

// NewEncryptedReader creates a reader that decrypts data on the fly
func NewEncryptedReader(reader io.Reader, encryptor *Encryptor) *EncryptedReader {
	return &EncryptedReader{
		reader:    reader,
		encryptor: encryptor,
	}
}

// EncryptedWriter wraps an io.Writer with encryption
type EncryptedWriter struct {
	writer    io.Writer
	encryptor *Encryptor
}

// NewEncryptedWriter creates a writer that encrypts data on the fly
func NewEncryptedWriter(writer io.Writer, encryptor *Encryptor) *EncryptedWriter {
	return &EncryptedWriter{
		writer:    writer,
		encryptor: encryptor,
	}
}

// Write encrypts and writes data
func (ew *EncryptedWriter) Write(p []byte) (int, error) {
	encrypted, err := ew.encryptor.Encrypt(p)
	if err != nil {
		return 0, err
	}

	// Write length prefix + encrypted data
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(encrypted)))

	if _, err := ew.writer.Write(lenBuf); err != nil {
		return 0, err
	}
	if _, err := ew.writer.Write(encrypted); err != nil {
		return 0, err
	}

	return len(p), nil
}

// ReadEncryptedFrame reads one encrypted frame from reader
func ReadEncryptedFrame(reader io.Reader, encryptor *Encryptor) ([]byte, error) {
	// Read length prefix
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return nil, err
	}

	frameLen := binary.BigEndian.Uint32(lenBuf)
	if frameLen > MaxPayloadSize+HeaderSize+100 {
		return nil, errors.New("frame too large")
	}

	// Read encrypted frame
	frame := make([]byte, frameLen)
	if _, err := io.ReadFull(reader, frame); err != nil {
		return nil, err
	}

	// Decrypt
	return encryptor.Decrypt(frame)
}

// WriteEncryptedFrame writes one encrypted frame to writer
func WriteEncryptedFrame(writer io.Writer, encryptor *Encryptor, data []byte) error {
	encrypted, err := encryptor.Encrypt(data)
	if err != nil {
		return err
	}

	// Write length prefix
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(encrypted)))

	if _, err := writer.Write(lenBuf); err != nil {
		return err
	}
	_, err = writer.Write(encrypted)
	return err
}
