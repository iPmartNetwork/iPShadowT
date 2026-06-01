package antidpi

import (
	"bytes"
	"crypto/ecdh"
	"encoding/hex"
	"io"
	"net"
	"testing"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

func TestRealityAuthRoundTrip(t *testing.T) {
	priv, pub, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	shortID, err := GenerateShortID()
	if err != nil {
		t.Fatal(err)
	}

	privBytes, _ := hex.DecodeString(priv)
	serverPriv, err := ecdh.X25519().NewPrivateKey(privBytes)
	if err != nil {
		t.Fatal(err)
	}
	pubBytes, _ := hex.DecodeString(pub)
	serverPub, err := ecdh.X25519().NewPublicKey(pubBytes)
	if err != nil {
		t.Fatal(err)
	}

	token, ephKey, err := BuildAuthToken(serverPub, shortID, 120)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := SerializeAuthToken(token)
	ephPub := ephKey.PublicKey().Bytes()

	ok, gotID := VerifyAuthToken(sessionID, ephPub, serverPriv, []string{shortID}, 120)
	if !ok {
		t.Fatal("VerifyAuthToken failed")
	}
	if gotID != shortID {
		t.Fatalf("short id mismatch: %s", gotID)
	}
}

func TestRealityClientHelloParse(t *testing.T) {
	priv, pub, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	shortID, err := GenerateShortID()
	if err != nil {
		t.Fatal(err)
	}

	pubBytes, _ := hex.DecodeString(pub)
	serverPub, _ := ecdh.X25519().NewPublicKey(pubBytes)

	client := &RealityClient{
		config: RealityConfig{
			ServerName:  "www.example.com",
			PublicKey:   pub,
			ShortID:     shortID,
			Fingerprint: "chrome",
		},
		publicKey: serverPub,
		utls:      NewUTLSDialer("chrome", "www.example.com", logger.New("error")),
	}

	server, clientConn := net.Pipe()
	defer server.Close()

	go func() {
		defer clientConn.Close()
		if err := client.sendAuthenticatedClientHello(clientConn); err != nil {
			t.Errorf("send hello: %v", err)
		}
	}()

	header := make([]byte, 5)
	if _, err := io.ReadFull(server, header); err != nil {
		t.Fatal(err)
	}
	if header[0] != 0x16 {
		t.Fatalf("expected TLS handshake record, got 0x%x", header[0])
	}
	recordLen := int(header[3])<<8 | int(header[4])
	record := make([]byte, recordLen)
	if _, err := io.ReadFull(server, record); err != nil {
		t.Fatal(err)
	}
	raw := append(header, record...)

	sessionID, ephPub, err := parseClientHelloAuth(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionID) != AuthTokenSize {
		t.Fatalf("session id len=%d", len(sessionID))
	}
	if len(ephPub) != 32 {
		t.Fatalf("eph pub len=%d", len(ephPub))
	}

	privBytes, _ := hex.DecodeString(priv)
	serverPriv, _ := ecdh.X25519().NewPrivateKey(privBytes)
	ok, _ := VerifyAuthToken(sessionID, ephPub, serverPriv, []string{shortID}, 120)
	if !ok {
		t.Fatal("parsed hello failed auth verify")
	}
}

func TestRealityServerHandleConnection(t *testing.T) {
	priv, pub, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	shortID, err := GenerateShortID()
	if err != nil {
		t.Fatal(err)
	}

	pubBytes, _ := hex.DecodeString(pub)
	serverPub, _ := ecdh.X25519().NewPublicKey(pubBytes)

	log := logger.New("error")
	client := &RealityClient{
		config: RealityConfig{
			ServerName:  "www.example.com",
			PublicKey:   pub,
			ShortID:     shortID,
			Fingerprint: "chrome",
		},
		publicKey: serverPub,
		utls:      NewUTLSDialer("chrome", "www.example.com", log),
		log:       log,
	}

	rs, err := NewRealityServer(RealityConfig{
		Dest:       "127.0.0.1:9",
		ServerName: "www.example.com",
		PrivateKey: priv,
		ShortIDs:   []string{shortID},
	}, logger.New("error"))
	if err != nil {
		t.Fatal(err)
	}

	server, clientConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, ok, err := rs.HandleConnection(server)
		if err != nil || !ok {
			t.Errorf("handle connection failed ok=%v err=%v", ok, err)
			return
		}
		defer conn.Close()
	}()

	conn, err := client.Connect(clientConn)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	<-done
}

func TestRealityAuthOKConstant(t *testing.T) {
	if len(RealityAuthOK) != 10 {
		t.Fatalf("unexpected auth ok length: %d", len(RealityAuthOK))
	}
	if !bytes.HasSuffix([]byte(RealityAuthOK), []byte("\n")) {
		t.Fatal("auth ok should end with newline")
	}
}

func TestVerifyAuthTokenRejectsReplay(t *testing.T) {
	priv, pub, _ := GenerateRealityKeyPair()
	shortID, _ := GenerateShortID()
	pubBytes, _ := hex.DecodeString(pub)
	serverPub, _ := ecdh.X25519().NewPublicKey(pubBytes)
	privBytes, _ := hex.DecodeString(priv)
	serverPriv, _ := ecdh.X25519().NewPrivateKey(privBytes)

	token, ephKey, _ := BuildAuthToken(serverPub, shortID, 120)
	sessionID := SerializeAuthToken(token)
	ephPub := ephKey.PublicKey().Bytes()

	rp := NewReplayProtection(time.Minute)
	if !rp.Check(sessionID) {
		t.Fatal("first token should be fresh")
	}
	if rp.Check(sessionID) {
		t.Fatal("replay should be rejected")
	}

	ok, _ := VerifyAuthToken(sessionID, ephPub, serverPriv, []string{shortID}, 120)
	if !ok {
		t.Fatal("token should still verify cryptographically")
	}
}
