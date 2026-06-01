package cli

import (
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
	"github.com/iPmart/iPShadowT/internal/utils"
)

// RunDoctor performs pre-flight checks for a config file.
func RunDoctor(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := logger.New("info")
	fmt.Println("iPShadowT doctor")
	fmt.Println("────────────────")

	fmt.Printf("  mode:       %s\n", cfg.Mode)
	fmt.Printf("  transport:  %s\n", cfg.Transport)
	fmt.Printf("  mux:        %v\n", cfg.Mux.Enabled)
	fmt.Printf("  profile:    %s\n", cfg.Performance.BufferProfile)
	fmt.Printf("  pool size:  %d\n", cfg.Pool.Size)
	fmt.Printf("  forwards:   %d\n", len(cfg.Forwards))
	fmt.Printf("  paths:      %d\n", len(cfg.Paths))

	if cfg.Mode == "client" && !cfg.Mux.Enabled {
		fmt.Println("  note:       direct mode (best upload) — server must also use mux.enabled = false")
	}

	if cfg.Mode == "server" && cfg.Mux.Enabled {
		fmt.Println("  note:       smux server — clients using direct mode will not connect")
	}

	if runtime.GOOS == "linux" && cfg.Performance.KernelTuning {
		kt := utils.NewKernelTuning(log)
		if err := kt.Apply(cfg.Performance.BufferProfile); err != nil {
			fmt.Printf("  kernel:     WARN (%v)\n", err)
		} else {
			fmt.Println("  kernel:     OK (tuning applied)")
		}
	} else if cfg.Performance.KernelTuning {
		fmt.Printf("  kernel:     skipped (%s)\n", runtime.GOOS)
	}

	if cfg.Mode == "client" {
		addr := cfg.RemoteAddr
		if addr == "" && len(cfg.Paths) > 0 {
			addr = cfg.Paths[0].RemoteAddr
		}
		if addr != "" {
			host, port, _ := net.SplitHostPort(addr)
			if host == "" {
				host = addr
			}
			if port != "" {
				fmt.Printf("  dns/tcp:    probing %s ...\n", addr)
				conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
				if err != nil {
					fmt.Printf("  tcp:        FAIL (%v)\n", err)
				} else {
					conn.Close()
					fmt.Println("  tcp:        OK")
				}
			}
		}
	}

	fmt.Println("────────────────")
	fmt.Println("Doctor finished.")
	return nil
}
