package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/iPmart/iPShadowT/internal/antidpi"
)

// handleKeyGen handles the --gen-reality-keys flag
func handleKeyGen() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  iPShadowT - REALITY Key Generator")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Generate X25519 key pair
	privateKey, publicKey, err := antidpi.GenerateRealityKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating keys: %v\n", err)
		os.Exit(1)
	}

	// Generate Short ID
	shortID, err := antidpi.GenerateShortID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating short ID: %v\n", err)
		os.Exit(1)
	}

	// Generate a random password
	passBuf := make([]byte, 24)
	rand.Read(passBuf)
	password := hex.EncodeToString(passBuf)

	fmt.Println("  🔑 REALITY Keys Generated:")
	fmt.Println()
	fmt.Printf("  Private Key: %s\n", privateKey)
	fmt.Printf("  Public Key:  %s\n", publicKey)
	fmt.Printf("  Short ID:    %s\n", shortID)
	fmt.Printf("  Password:    %s\n", password)
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  📋 Server Config (server-reality.toml):")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Printf("  password = \"%s\"\n", password)
	fmt.Println()
	fmt.Println("  [reality]")
	fmt.Printf("  private_key = \"%s\"\n", privateKey)
	fmt.Printf("  short_id = \"%s\"\n", shortID)
	fmt.Println("  server_name = \"www.google.com\"")
	fmt.Println("  dest = \"www.google.com:443\"")
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  📋 Client Config (client-reality.toml):")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Printf("  password = \"%s\"\n", password)
	fmt.Println()
	fmt.Println("  [reality]")
	fmt.Printf("  public_key = \"%s\"\n", publicKey)
	fmt.Printf("  short_id = \"%s\"\n", shortID)
	fmt.Println("  server_name = \"www.google.com\"")
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  ⚠️  Keep the Private Key SECRET!")
	fmt.Println("  ⚠️  Share Public Key + Short ID with clients")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
