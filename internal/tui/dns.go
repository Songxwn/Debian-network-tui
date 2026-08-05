package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/debian-network-tui/debian-network-tui/internal/packages"
	"github.com/debian-network-tui/debian-network-tui/internal/resolvconf"
)

const (
	dnsNS1 = iota
	dnsNS2
	dnsNS3
	dnsSearch
	dnsFieldCount
)

func (m *Model) startDNSForm() {
	path := resolvconf.DefaultPath
	if v := os.Getenv("RESOLV_CONF"); v != "" {
		path = v
	}
	cfg, err := resolvconf.Load(path)
	m.dnsPath = path
	m.dnsSymlink = ""
	m.dnsWarn = ""
	m.status = ""
	if err != nil {
		m.dnsWarn = err.Error()
		cfg = &resolvconf.Config{Path: path}
	}
	if cfg.SymlinkTarget != "" {
		m.dnsSymlink = cfg.SymlinkTarget
		m.dnsWarn = "Warning: resolv.conf is a symlink → " + cfg.SymlinkTarget + "\nSaving will replace it with a regular file."
	}

	vals := [dnsFieldCount]string{}
	for i := 0; i < 3 && i < len(cfg.Nameservers); i++ {
		vals[i] = cfg.Nameservers[i]
	}
	vals[dnsSearch] = strings.Join(cfg.Search, " ")

	placeholders := [dnsFieldCount]string{
		"nameserver e.g. 8.8.8.8",
		"optional nameserver",
		"optional nameserver",
		"search domains, space-separated",
	}
	m.dnsInputs = make([]textinput.Model, dnsFieldCount)
	for i := 0; i < dnsFieldCount; i++ {
		ti := textinput.New()
		ti.Placeholder = placeholders[i]
		ti.SetValue(vals[i])
		ti.CharLimit = 128
		ti.Width = 40
		m.dnsInputs[i] = ti
	}
	m.dnsFocus = dnsNS1
	m.syncDNSFocus()
	m.screen = screenDNS
}

func (m *Model) beginApplyDNSFromFile() {
	dir, err := packages.SelfDir()
	if err != nil {
		m.showMsg("Apply DNS failed", err.Error(), screenMenu)
		return
	}
	files, err := resolvconf.FindLocalConfigs(dir)
	if err != nil {
		m.showMsg("Apply DNS failed", err.Error(), screenMenu)
		return
	}
	if len(files) == 0 {
		m.showMsg("No DNS config file",
			fmt.Sprintf("Searched: %s\n\nPlace one of:\n  resolv.conf\n  dns-resolv.conf\n  dns.conf\nnext to this binary.\nSee examples/resolv.conf", dir),
			screenMenu)
		return
	}
	dest := resolvconf.DefaultPath
	if v := os.Getenv("RESOLV_CONF"); v != "" {
		dest = v
	}
	m.pendingDNSDir = dir
	m.pendingDNSFiles = files
	m.pendingDNSDest = dest
	m.pendingDNSBack = m.screen
	m.confirm = confirmApplyDNSFile
	m.screen = screenConfirm
}

func (m *Model) syncDNSFocus() {
	for i := range m.dnsInputs {
		if i == m.dnsFocus {
			m.dnsInputs[i].Focus()
		} else {
			m.dnsInputs[i].Blur()
		}
	}
}

func (m Model) updateDNSForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenMenu
		m.status = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+s":
		m.confirm = confirmSaveDNS
		m.screen = screenConfirm
		return m, nil
	case "ctrl+o":
		m.beginApplyDNSFromFile()
		return m, nil
	case "tab", "down":
		m.dnsFocus = (m.dnsFocus + 1) % dnsFieldCount
		m.syncDNSFocus()
		return m, nil
	case "shift+tab", "up":
		m.dnsFocus = (m.dnsFocus - 1 + dnsFieldCount) % dnsFieldCount
		m.syncDNSFocus()
		return m, nil
	}
	var cmd tea.Cmd
	m.dnsInputs[m.dnsFocus], cmd = m.dnsInputs[m.dnsFocus].Update(msg)
	return m, cmd
}

func (m Model) viewDNSForm() string {
	var b strings.Builder
	b.WriteString(sectionStyle.Render("Edit DNS (/etc/resolv.conf)") + "\n\n")
	b.WriteString(subtleStyle.Render("  File: "+m.dnsPath) + "\n")
	if m.dnsWarn != "" {
		b.WriteString(errorStyle.Render("  "+m.dnsWarn) + "\n")
	}
	b.WriteString("\n")

	row := func(focus int, label, value string) string {
		cursor := "  "
		style := itemStyle
		if m.dnsFocus == focus {
			cursor = "> "
			style = selectedStyle
		}
		return style.Render(fmt.Sprintf("%s%-14s %s", cursor, label, value)) + "\n"
	}
	b.WriteString(row(dnsNS1, "Nameserver 1", m.dnsInputs[dnsNS1].View()))
	b.WriteString(row(dnsNS2, "Nameserver 2", m.dnsInputs[dnsNS2].View()))
	b.WriteString(row(dnsNS3, "Nameserver 3", m.dnsInputs[dnsNS3].View()))
	b.WriteString(row(dnsSearch, "Search", m.dnsInputs[dnsSearch].View()))
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  Ctrl+S save  |  Ctrl+O overwrite from local file") + "\n")
	return b.String()
}

func (m *Model) buildDNSFromForm() (*resolvconf.Config, error) {
	cfg := &resolvconf.Config{Path: m.dnsPath}
	for i := dnsNS1; i <= dnsNS3; i++ {
		ns := strings.TrimSpace(m.dnsInputs[i].Value())
		if ns != "" {
			cfg.Nameservers = append(cfg.Nameservers, ns)
		}
	}
	search := strings.Fields(strings.TrimSpace(m.dnsInputs[dnsSearch].Value()))
	cfg.Search = search
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
