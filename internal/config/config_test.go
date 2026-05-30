package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidServerConfig(t *testing.T) {
	content := `
mode = "server"
transport = "tcpmux"
bind_addr = "0.0.0.0:443"
password = "test-password"
log_level = "info"

[mux]
concurrency = 8

[performance]
nodelay = true
`
	cfg := writeAndLoad(t, content)

	if cfg.Mode != "server" {
		t.Fatalf("expected mode=server, got %s", cfg.Mode)
	}
	if cfg.BindAddr != "0.0.0.0:443" {
		t.Fatalf("expected bind_addr=0.0.0.0:443, got %s", cfg.BindAddr)
	}
	if cfg.Mux.Concurrency != 8 {
		t.Fatalf("expected mux.concurrency=8, got %d", cfg.Mux.Concurrency)
	}
}

func TestLoadValidClientConfig(t *testing.T) {
	content := `
mode = "client"
transport = "wsmux"
remote_addr = "server.com:443"
password = "test-password"

[[forwards]]
name = "socks5"
type = "socks5"
listen = "127.0.0.1:1080"
`
	cfg := writeAndLoad(t, content)

	if cfg.Mode != "client" {
		t.Fatalf("expected mode=client, got %s", cfg.Mode)
	}
	if len(cfg.Forwards) != 1 {
		t.Fatalf("expected 1 forward, got %d", len(cfg.Forwards))
	}
	if cfg.Forwards[0].Type != "socks5" {
		t.Fatalf("expected forward type=socks5, got %s", cfg.Forwards[0].Type)
	}
}

func TestLoadInvalidMode(t *testing.T) {
	content := `
mode = "invalid"
password = "test"
`
	_, err := loadFromString(content)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestLoadMissingPassword(t *testing.T) {
	content := `
mode = "server"
bind_addr = "0.0.0.0:443"
`
	_, err := loadFromString(content)
	if err == nil {
		t.Fatal("expected error for missing password")
	}
}

func TestDefaults(t *testing.T) {
	content := `
mode = "server"
bind_addr = "0.0.0.0:443"
password = "test"
`
	cfg := writeAndLoad(t, content)

	// Check defaults are applied
	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log_level=info, got %s", cfg.LogLevel)
	}
	if cfg.Transport != "tcpmux" {
		t.Fatalf("expected default transport=tcpmux, got %s", cfg.Transport)
	}
	if cfg.Mux.Concurrency != 8 {
		t.Fatalf("expected default mux.concurrency=8, got %d", cfg.Mux.Concurrency)
	}
	if cfg.Heartbeat.Interval != 20 {
		t.Fatalf("expected default heartbeat.interval=20, got %d", cfg.Heartbeat.Interval)
	}
}

func writeAndLoad(t *testing.T, content string) *Config {
	t.Helper()
	cfg, err := loadFromString(content)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	return cfg
}

func loadFromString(content string) (*Config, error) {
	dir := os.TempDir()
	path := filepath.Join(dir, "test-config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, err
	}
	defer os.Remove(path)
	return Load(path)
}
