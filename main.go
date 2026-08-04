package main

import (
	"fmt"
	"os"
	"os/user"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/debian-network-tui/debian-network-tui/internal/tui"
)

// Set by CI via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println("debian-network-tui", version)
		return
	}

	if err := requireRoot(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\nRun as root: sudo debian-network-tui\n\n", err)
	}

	cfgPath := "/etc/network/interfaces"
	if v := os.Getenv("INTERFACES_FILE"); v != "" {
		cfgPath = v
	}

	m := tui.New(cfgPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
		os.Exit(1)
	}
}

func requireRoot() error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("cannot determine current user: %w", err)
	}
	if u.Uid != "0" {
		return fmt.Errorf("not running as root (uid=%s)", u.Uid)
	}
	return nil
}
