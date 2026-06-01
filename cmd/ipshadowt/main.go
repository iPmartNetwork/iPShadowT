package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iPmart/iPShadowT/internal/cli"
	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/client"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/server"
)

var (
	Version   = "v2.2.2"
	Commit    = "unknown"
	BuildTime = "unknown"
	Author    = "iPmart Network (Ali Hassanzadeh)"
	Project   = "iPShadowT"
)

type stoppable interface {
	Stop()
}

func main() {
	configPath := flag.String("c", "", "Path to config file (e.g., -c server.toml)")
	showVersion := flag.Bool("v", false, "Show version")
	genKeys := flag.Bool("gen-reality-keys", false, "Generate REALITY key pair")
	validateOnly := flag.Bool("validate", false, "Validate config file and exit")
	doctor := flag.Bool("doctor", false, "Run pre-flight diagnostics on config")
	flag.Parse()

	if *showVersion {
		fmt.Printf("iPShadowT %s (built: %s)\n", Version, BuildTime)
		fmt.Printf("  Author: %s\n", Author)
		fmt.Println("  iP: iPmart | Shadow: Stealth | T: Tunnel")
		fmt.Println("  Anti-DPI Multi-Transport Tunnel")
		fmt.Printf("  https://github.com/iPmartNetwork/%s\n", Project)
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
		fmt.Printf("  %s -c server.toml       # Run as server\n", os.Args[0])
		fmt.Printf("  %s -c client.toml       # Run as client\n", os.Args[0])
		fmt.Printf("  %s -validate -c x.toml  # Validate config\n", os.Args[0])
		fmt.Printf("  %s -doctor -c x.toml    # Pre-flight checks\n", os.Args[0])
		fmt.Printf("  %s -v                    # Show version\n", os.Args[0])
		os.Exit(1)
	}

	if *validateOnly {
		if err := cli.ValidateConfig(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if *doctor {
		if err := cli.RunDoctor(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Doctor failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	log := logger.NewWithFormat(cfg.LogLevel, cfg.LogFormat)

	log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info("  iPShadowT %s", Version)
	log.Info("  Author: %s", Author)
	log.Info("  Anti-DPI Multi-Transport Tunnel")
	log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info("  Mode:        %s", cfg.Mode)
	log.Info("  Transport:   %s", cfg.Transport)
	log.Info("  Mux:         %v", cfg.Mux.Enabled)
	log.Info("  Log Level:   %s", cfg.LogLevel)
	log.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var runner stoppable

	switch cfg.Mode {
	case "server":
		srv, err := server.New(cfg, log)
		if err != nil {
			log.Fatal("Failed to create server: %v", err)
		}
		if err := srv.Start(); err != nil {
			log.Fatal("Failed to start server: %v", err)
		}
		runner = srv
		log.Info("🟢 Server started successfully")

	case "client":
		cliInst, err := client.New(cfg, log)
		if err != nil {
			log.Fatal("Failed to create client: %v", err)
		}
		if err := cliInst.Start(); err != nil {
			log.Fatal("Failed to start client: %v", err)
		}
		runner = cliInst
		log.Info("🟢 Client connected successfully")

	default:
		log.Fatal("Unknown mode: %s (use 'server' or 'client')", cfg.Mode)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("🔴 Shutting down...")
	if runner != nil {
		done := make(chan struct{})
		go func() {
			runner.Stop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			log.Warn("Shutdown timed out after 15s")
		}
	}
	log.Info("Goodbye.")
}
