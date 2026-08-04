package main

import (
	"fmt"
	"os"
	"os/user"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/debian-network-tui/debian-network-tui/internal/tui"
)

// Set by goreleaser / CI via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println("debian-network-tui", version)
		return
	}

	if err := requireRoot(); err != nil {
		fmt.Fprintf(os.Stderr, "警告: %v\n建议使用 root 权限运行: sudo debian-network-tui\n\n", err)
	}

	cfgPath := "/etc/network/interfaces"
	if v := os.Getenv("INTERFACES_FILE"); v != "" {
		cfgPath = v
	}

	m := tui.New(cfgPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "程序异常退出: %v\n", err)
		os.Exit(1)
	}
}

func requireRoot() error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("无法获取当前用户: %w", err)
	}
	if u.Uid != "0" {
		return fmt.Errorf("当前用户不是 root (uid=%s)", u.Uid)
	}
	return nil
}
