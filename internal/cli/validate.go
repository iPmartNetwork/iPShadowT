package cli

import (
	"fmt"

	"github.com/iPmart/iPShadowT/internal/config"
)

// ValidateConfig loads and validates a config file, returning warnings and errors.
func ValidateConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	fmt.Printf("OK: %s (mode=%s transport=%s mux=%v)\n", path, cfg.Mode, cfg.Transport, cfg.Mux.Enabled)
	return nil
}
