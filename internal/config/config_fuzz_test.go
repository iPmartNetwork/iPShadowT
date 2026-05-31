package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

// FuzzConfigParse tests that the config parser doesn't panic on arbitrary input
// Run: go test -fuzz=FuzzConfigParse -fuzztime=60s ./internal/config/
func FuzzConfigParse(f *testing.F) {
	// Seed with valid configs
	f.Add([]byte(`mode = "server"
transport = "tcpmux"
bind_addr = "0.0.0.0:443"
password = "test123"
`))
	f.Add([]byte(`mode = "client"
transport = "reality"
remote_addr = "1.2.3.4:443"
password = "secret"

[mux]
concurrency = 4
`))
	f.Add([]byte(`invalid toml {{{{`))
	f.Add([]byte(``))
	f.Add([]byte(`mode = ""`))

	f.Fuzz(func(t *testing.T, data []byte) {
		cfg := &Config{}
		// Should not panic regardless of input
		_ = toml.Unmarshal(data, cfg)

		// If it parsed, try applying defaults (should not panic)
		applyDefaults(cfg)

		// Validate may return error but should not panic
		_ = validate(cfg)
	})
}
