package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/debian-network-tui/debian-network-tui/internal/bootstrap"
	"github.com/debian-network-tui/debian-network-tui/internal/packages"
)

type bootLogMsg struct {
	lines    []string
	nextStep int
	err      error
	done     bool
}

func (m *Model) beginOneShotSetup() {
	dir, err := packages.SelfDir()
	if err != nil {
		m.showMsg("One-shot setup failed", err.Error(), screenMenu)
		return
	}
	plan, err := bootstrap.Prepare(dir)
	if err != nil {
		m.showMsg("One-shot setup not ready",
			err.Error()+"\n\nPlace beside the binary:\n"+
				"  resolv.conf (or dns.conf)\n"+
				"  sources.list / *.list / *.sources\n"+
				"  ifenslave_*.deb + vlan_*.deb\n"+
				"  root.pub (or ssh-root.conf)\n"+
				"See examples/",
			screenMenu)
		return
	}
	m.pendingBoot = plan
	m.confirm = confirmOneShot
	m.screen = screenConfirm
}

func bootStepCmd(plan *bootstrap.Plan, step int) tea.Cmd {
	return func() tea.Msg {
		lines, next, err := plan.ExecuteStep(step)
		if err != nil {
			return bootLogMsg{lines: lines, nextStep: next, err: err, done: true}
		}
		if next >= bootstrap.StepCount {
			return bootLogMsg{lines: lines, nextStep: next, done: true}
		}
		return bootLogMsg{lines: lines, nextStep: next, done: false}
	}
}

func (m Model) handleBootLogMsg(msg bootLogMsg) (tea.Model, tea.Cmd) {
	m.touch()
	m.bootLog = append(m.bootLog, msg.lines...)
	if msg.err != nil {
		m.bootErr = msg.err
		m.bootDone = true
		m.bootLog = append(m.bootLog, "", "STOPPED: "+msg.err.Error())
		return m, nil
	}
	if msg.done {
		m.bootDone = true
		return m, nil
	}
	return m, bootStepCmd(m.pendingBoot, msg.nextStep)
}

func (m Model) updateBootLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.bootDone {
		// Allow quit only via ctrl+c while running (dangerous mid-apt).
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	switch msg.String() {
	case "enter", "esc", "q", " ":
		m.screen = screenMenu
		m.pendingBoot = nil
		m.bootLog = nil
		m.bootErr = nil
		m.bootDone = false
		m.status = ""
	case "up", "k":
		if m.bootScroll > 0 {
			m.bootScroll--
		}
	case "down", "j":
		maxScroll := len(m.bootLog) - 8
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.bootScroll < maxScroll {
			m.bootScroll++
		}
	}
	return m, nil
}

func (m Model) viewBootLog() string {
	var b strings.Builder
	title := "One-shot setup — running…"
	if m.bootDone {
		if m.bootErr != nil {
			title = "One-shot setup — FAILED"
		} else {
			title = "One-shot setup — done"
		}
	}
	b.WriteString(sectionStyle.Render(title) + "\n\n")

	maxBody := 18
	if m.height > 12 {
		maxBody = m.height - 10
		if maxBody < 8 {
			maxBody = 8
		}
		if maxBody > 40 {
			maxBody = 40
		}
	}
	lines := m.bootLog
	total := len(lines)
	start := 0
	if total > maxBody {
		if m.bootDone {
			start = m.bootScroll
			if start < 0 {
				start = 0
			}
			if start > total-maxBody {
				start = total - maxBody
			}
		} else {
			start = total - maxBody
		}
	}
	end := start + maxBody
	if end > total {
		end = total
	}
	for _, ln := range lines[start:end] {
		style := itemStyle
		switch {
		case strings.HasPrefix(ln, "==>"):
			style = selectedStyle
		case strings.Contains(ln, "FAIL") || strings.HasPrefix(ln, "STOPPED"):
			style = errorStyle
		case strings.HasPrefix(ln, "All steps"):
			style = statusStyle
		}
		b.WriteString(style.Render(ln) + "\n")
	}
	if total > maxBody {
		b.WriteString("\n" + subtleStyle.Render(fmt.Sprintf("  (lines %d–%d of %d)", start+1, end, total)) + "\n")
	}
	if !m.bootDone {
		b.WriteString("\n" + subtleStyle.Render("  Working… please wait (Ctrl+C to quit)") + "\n")
	} else {
		b.WriteString("\n" + subtleStyle.Render("  Enter/Esc return to menu") + "\n")
	}
	return b.String()
}
