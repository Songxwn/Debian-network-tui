package tui

import (
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Default idle timeout before automatic exit.
const defaultIdleTimeout = 65 * time.Second

type idleTickMsg time.Time

func tickIdle() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return idleTickMsg(t)
	})
}

func idleTimeoutFromEnv() time.Duration {
	v := os.Getenv("IDLE_TIMEOUT_SEC")
	if v == "" {
		return defaultIdleTimeout
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultIdleTimeout
	}
	if n <= 0 {
		return 0 // disabled
	}
	return time.Duration(n) * time.Second
}

func (m *Model) touch() {
	m.lastActivity = time.Now()
}

func (m Model) idleRemaining() time.Duration {
	if m.idleTimeout <= 0 {
		return 0
	}
	left := m.idleTimeout - time.Since(m.lastActivity)
	if left < 0 {
		return 0
	}
	return left
}

// IdleTimedOut reports whether the program exited due to idle timeout.
func (m Model) IdleTimedOut() bool {
	return m.idleTimedOut
}
