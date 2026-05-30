package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/client"
	"github.com/iPmart/iPShadowT/internal/server"
	"github.com/iPmart/iPShadowT/internal/logger"
)

var (
	Version   = "v1.0.0-alpha1"
	BuildTime = "unknown"
	Author    = "iPmart Network (Ali Hassanzadeh)"
	Project   = "iPShadowT"
)

func main() {
	configPath := flag.String("c", "", "Path to config file (e.g., -c server.toml)")
	showVersion := flag.Bool("v", false, "Show version")
	genKeys := flag.Bool("gen-reality-keys", false, "Generate REALITY key pair")
	flag.Parse()

	if *showVersion {
		fmt.Printf("iPShadowT %s (built: %s)\n", Version, BuildTime)
		fmt.Printf("  Author: %s\n", Author)
		fmt.Println("  iP: iPmart | Shadow: Stealth | T: Tunnel")
		fmt.Println("  Anti-DPI Multi-Transport Tunnel")
		fmt.Printf("  https://github.com/iPmart/%s\n", Project)
		os.Exit(0)
	}

	if *genKeys {
		handleKeyGen()
		os.Exit(0)
	}

	if *configPath == "" {
		fmt.Println("iPShadowT - Anti-DPI Multi-Transport Tunnel")
		fmt.Println()
		fmt.Printf("Usage: %s -c <config.toml>\n", os.Args[0])
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Printf("  %s -c server.toml    # Run as server\n", os.Args[0])
		fmt.Printf("  %s -c client.toml    # Run as client\n", os.Args[0])
		fmt.Printf("  %s -v               # Show version\n", os.Args[0])
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.LogLevel)

	log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info("  iPShadowT %s", Version)
	log.Info("  Author: %s", Author)
	log.Info("  Anti-DPI Multi-Transport Tunnel")
	log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info("  Mode:        %s", cfg.Mode)
	log.Info("  Transport:   %s", cfg.Transport)
	log.Info("  Log Level:   %s", cfg.LogLevel)
	log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Start based on mode
	switch cfg.Mode {
	case "server":
		srv, err := server.New(cfg, log)
		if err != nil {
			log.Fatal("Failed to create server: %v", err)
		}
		if err := srv.Start(); err != nil {
			log.Fatal("Failed to start server: %v", err)
		}
		log.Info("🟢 Server started successfully")

	case "client":
		cli, err := client.New(cfg, log)
		if err != nil {
			log.Fatal("Failed to create client: %v", err)
		}
		if err := cli.Start(); err != nil {
			log.Fatal("Failed to start client: %v", err)
		}
		log.Info("🟢 Client connected successfully")

	default:
		log.Fatal("Unknown mode: %s (use 'server' or 'client')", cfg.Mode)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("🔴 Shutting down...")
}
