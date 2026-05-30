package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Interactive provides an interactive CLI menu for setup and management
type Interactive struct {
	log    *logger.Logger
	reader *bufio.Reader
}

// NewInteractive creates a new interactive CLI
func NewInteractive(log *logger.Logger) *Interactive {
	return &Interactive{
		log:    log,
		reader: bufio.NewReader(os.Stdin),
	}
}

// RunSetupWizard runs the initial setup wizard
func (i *Interactive) RunSetupWizard() (map[string]string, error) {
	result := make(map[string]string)

	i.printBanner()

	// Mode selection
	fmt.Println("\n┌─ Select Mode ─────────────────────────────┐")
	fmt.Println("│  1) Server (run on VPS abroad)             │")
	fmt.Println("│  2) Client (run on Iran server/local)      │")
	fmt.Println("└────────────────────────────────────────────┘")
	mode := i.ask("Choice [1/2]", "1")
	if mode == "1" {
		result["mode"] = "server"
	} else {
		result["mode"] = "client"
	}

	// Transport selection
	fmt.Println("\n┌─ Select Transport ────────────────────────┐")
	fmt.Println("│  1) tcpmux     - Fast, simple              │")
	fmt.Println("│  2) wsmux      - CDN compatible            │")
	fmt.Println("│  3) reality    - Maximum stealth ⭐        │")
	fmt.Println("│  4) shadowtls  - No cert needed            │")
	fmt.Println("│  5) h2mux      - Looks like web traffic    │")
	fmt.Println("│  6) grpc       - Looks like API calls      │")
	fmt.Println("└────────────────────────────────────────────┘")
	transport := i.ask("Choice [1-6]", "3")
	transports := map[string]string{"1": "tcpmux", "2": "wsmux", "3": "reality", "4": "shadowtls", "5": "h2mux", "6": "grpc"}
	result["transport"] = transports[transport]
	if result["transport"] == "" {
		result["transport"] = "reality"
	}

	// Address
	if result["mode"] == "server" {
		result["bind_addr"] = i.ask("Bind address", "0.0.0.0:443")
	} else {
		result["remote_addr"] = i.ask("Server address (ip:port)", "")
	}

	// Password
	result["password"] = i.ask("Password (shared secret)", "")

	// Anti-DPI
	if result["mode"] == "client" {
		antiDPI := i.ask("Enable Anti-DPI? [y/n]", "y")
		result["anti_dpi"] = antiDPI

		if antiDPI == "y" {
			fmt.Println("\n┌─ uTLS Fingerprint ────────────────────────┐")
			fmt.Println("│  1) chrome    - Most common               │")
			fmt.Println("│  2) firefox   - Alternative               │")
			fmt.Println("│  3) safari    - Apple devices             │")
			fmt.Println("│  4) random    - Randomized                │")
			fmt.Println("└────────────────────────────────────────────┘")
			fp := i.ask("Choice [1-4]", "1")
			fps := map[string]string{"1": "chrome", "2": "firefox", "3": "safari", "4": "random"}
			result["fingerprint"] = fps[fp]
		}

		// SOCKS5
		socks := i.ask("SOCKS5 proxy address", "127.0.0.1:1080")
		result["socks5"] = socks
	}

	return result, nil
}

// RunMainMenu shows the main management menu
func (i *Interactive) RunMainMenu() string {
	fmt.Println("\n┌─ iPShadowT Management ────────────────────┐")
	fmt.Println("│  1) Start tunnel                           │")
	fmt.Println("│  2) Stop tunnel                            │")
	fmt.Println("│  3) Status                                 │")
	fmt.Println("│  4) View logs                              │")
	fmt.Println("│  5) Manage users                           │")
	fmt.Println("│  6) Speed test                             │")
	fmt.Println("│  7) Network diagnostics                    │")
	fmt.Println("│  8) Generate REALITY keys                  │")
	fmt.Println("│  9) Edit config                            │")
	fmt.Println("│  0) Exit                                   │")
	fmt.Println("└────────────────────────────────────────────┘")
	return i.ask("Choice", "3")
}

func (i *Interactive) ask(prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Printf("  %s: ", prompt)
	}

	input, _ := i.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}
	return input
}

func (i *Interactive) printBanner() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  🛡️  iPShadowT Setup Wizard")
	fmt.Println("  iPmart Network (Ali Hassanzadeh)")
	fmt.Println("  Anti-DPI Multi-Transport Tunnel")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
