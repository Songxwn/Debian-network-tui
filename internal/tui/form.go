package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/debian-network-tui/debian-network-tui/internal/interfaces"
	"github.com/debian-network-tui/debian-network-tui/internal/netdev"
)

// Form field indices for text inputs after toggle fields.
// Layout focus order:
// 0 name, 1 auto, 2 hotplug, 3 ipv4 method, 4 address, 5 netmask, 6 gateway, 7 dns,
// 8 ipv6 method, 9 v6 address, 10 v6 gateway
const (
	fName = iota
	fAuto
	fHotplug
	fIPv4Method
	fAddress
	fNetmask
	fGateway
	fDNS
	fIPv6Method
	fV6Address
	fV6Gateway
	fCount
)

var ipv4Methods = []string{"dhcp", "static", "disabled"}
var ipv6Methods = []string{"disabled", "dhcp", "static", "auto"}

func (m *Model) startNewForm() {
	c := interfaces.NewConnection("")
	m.startEditForm(c, true)
}

func (m *Model) startEditForm(c *interfaces.Connection, isNew bool) {
	m.editConn = c
	m.editNew = isNew
	m.autoOn = c.Auto
	m.hotplugOn = c.AllowHotplug

	m.ipv4Method = 2 // disabled
	if c.IPv4 != nil {
		switch c.IPv4.Method {
		case interfaces.MethodDHCP:
			m.ipv4Method = 0
		case interfaces.MethodStatic:
			m.ipv4Method = 1
		default:
			m.ipv4Method = 0
		}
	}
	m.ipv6Method = 0
	if c.IPv6 != nil {
		switch c.IPv6.Method {
		case interfaces.MethodDHCP:
			m.ipv6Method = 1
		case interfaces.MethodStatic:
			m.ipv6Method = 2
		case interfaces.MethodManual:
			m.ipv6Method = 3
		default:
			m.ipv6Method = 1
		}
	}

	m.inputs = make([]textinput.Model, 5) // name, v4addr, mask, gw, dns, + v6addr, v6gw mapped separately
	// We'll use a map-like slice of 7 text fields: name, addr, mask, gw, dns, v6addr, v6gw
	labels := []struct {
		placeholder string
		value       string
		width       int
	}{
		{"iface name, e.g. eth0 / ens18", c.Name, 24},
		{"IPv4 address", "", 24},
		{"netmask, e.g. 255.255.255.0", "", 24},
		{"default gateway", "", 24},
		{"DNS servers, space-separated", "", 40},
		{"IPv6 address/prefix, e.g. 2001:db8::1/64", "", 40},
		{"IPv6 gateway", "", 40},
	}
	if c.IPv4 != nil {
		labels[1].value = c.IPv4.GetOption("address")
		labels[2].value = c.IPv4.GetOption("netmask")
		labels[3].value = c.IPv4.GetOption("gateway")
		labels[4].value = c.IPv4.GetOption("dns-nameservers")
	}
	if c.IPv6 != nil {
		labels[5].value = c.IPv6.GetOption("address")
		labels[6].value = c.IPv6.GetOption("gateway")
	}

	m.inputs = make([]textinput.Model, len(labels))
	for i, l := range labels {
		ti := textinput.New()
		ti.Placeholder = l.placeholder
		ti.SetValue(l.value)
		ti.CharLimit = 128
		ti.Width = l.width
		m.inputs[i] = ti
	}
	m.formFocus = fName
	m.syncInputFocus()
	m.screen = screenEditForm
	m.status = ""
}

// inputIndex maps form focus to textinput index; -1 if not a text field.
func inputIndex(focus int) int {
	switch focus {
	case fName:
		return 0
	case fAddress:
		return 1
	case fNetmask:
		return 2
	case fGateway:
		return 3
	case fDNS:
		return 4
	case fV6Address:
		return 5
	case fV6Gateway:
		return 6
	default:
		return -1
	}
}

func (m *Model) syncInputFocus() {
	idx := inputIndex(m.formFocus)
	for i := range m.inputs {
		if i == idx {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m Model) updateEditForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenEditList
		m.status = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+s":
		m.confirm = confirmSave
		m.screen = screenConfirm
		return m, nil
	case "tab", "down":
		m.formFocus = (m.formFocus + 1) % fCount
		m.syncInputFocus()
		return m, nil
	case "shift+tab", "up":
		m.formFocus = (m.formFocus - 1 + fCount) % fCount
		m.syncInputFocus()
		return m, nil
	case "left", "right", " ", "enter":
		isToggle := m.formFocus == fAuto || m.formFocus == fHotplug ||
			m.formFocus == fIPv4Method || m.formFocus == fIPv6Method
		if !isToggle {
			break
		}
		switch m.formFocus {
		case fAuto:
			m.autoOn = !m.autoOn
		case fHotplug:
			m.hotplugOn = !m.hotplugOn
		case fIPv4Method:
			if msg.String() == "left" {
				m.ipv4Method = (m.ipv4Method - 1 + len(ipv4Methods)) % len(ipv4Methods)
			} else {
				m.ipv4Method = (m.ipv4Method + 1) % len(ipv4Methods)
			}
		case fIPv6Method:
			if msg.String() == "left" {
				m.ipv6Method = (m.ipv6Method - 1 + len(ipv6Methods)) % len(ipv6Methods)
			} else {
				m.ipv6Method = (m.ipv6Method + 1) % len(ipv6Methods)
			}
		}
		return m, nil
	}

	// Forward to focused text input
	if idx := inputIndex(m.formFocus); idx >= 0 {
		var cmd tea.Cmd
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) viewEditForm() string {
	var b strings.Builder
	title := "Edit connection"
	if m.editNew {
		title = "Add connection"
	}
	b.WriteString(sectionStyle.Render(title) + "\n\n")

	row := func(focus int, label, value string) string {
		cursor := "  "
		style := itemStyle
		if m.formFocus == focus {
			cursor = "> "
			style = selectedStyle
		}
		return style.Render(fmt.Sprintf("%s%-14s %s", cursor, label, value)) + "\n"
	}

	boolStr := func(v bool) string {
		if v {
			return "[x] Yes"
		}
		return "[ ] No"
	}

	b.WriteString(row(fName, "Device", m.inputs[0].View()))
	b.WriteString(row(fAuto, "Auto start", boolStr(m.autoOn)))
	b.WriteString(row(fHotplug, "Hotplug", boolStr(m.hotplugOn)))
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  —— IPv4 ——") + "\n")
	b.WriteString(row(fIPv4Method, "Method", "< "+ipv4Methods[m.ipv4Method]+" >"))
	b.WriteString(row(fAddress, "Address", m.inputs[1].View()))
	b.WriteString(row(fNetmask, "Netmask", m.inputs[2].View()))
	b.WriteString(row(fGateway, "Gateway", m.inputs[3].View()))
	b.WriteString(row(fDNS, "DNS", m.inputs[4].View()))
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  —— IPv6 ——") + "\n")
	b.WriteString(row(fIPv6Method, "Method", "< "+ipv6Methods[m.ipv6Method]+" >"))
	b.WriteString(row(fV6Address, "Address", m.inputs[5].View()))
	b.WriteString(row(fV6Gateway, "Gateway", m.inputs[6].View()))
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  Ctrl+S to save (original file is backed up automatically)") + "\n")
	return b.String()
}

func (m *Model) buildConnFromForm() (*interfaces.Connection, error) {
	name := strings.TrimSpace(m.inputs[0].Value())
	if name == "" {
		return nil, fmt.Errorf("interface name is required")
	}
	c := &interfaces.Connection{
		Name:         name,
		Auto:         m.autoOn,
		AllowHotplug: m.hotplugOn,
	}

	switch m.ipv4Method {
	case 0: // dhcp
		c.EnsureIPv4(interfaces.MethodDHCP)
	case 1: // static
		c.EnsureIPv4(interfaces.MethodStatic)
		c.IPv4.SetOption("address", strings.TrimSpace(m.inputs[1].Value()))
		c.IPv4.SetOption("netmask", strings.TrimSpace(m.inputs[2].Value()))
		c.IPv4.SetOption("gateway", strings.TrimSpace(m.inputs[3].Value()))
		c.IPv4.SetOption("dns-nameservers", strings.TrimSpace(m.inputs[4].Value()))
	case 2: // disabled
		c.ClearIPv4()
	}

	switch m.ipv6Method {
	case 0:
		c.ClearIPv6()
	case 1:
		c.EnsureIPv6(interfaces.MethodDHCP)
	case 2:
		c.EnsureIPv6(interfaces.MethodStatic)
		c.IPv6.SetOption("address", strings.TrimSpace(m.inputs[5].Value()))
		c.IPv6.SetOption("gateway", strings.TrimSpace(m.inputs[6].Value()))
	case 3: // auto -> manual (SLAAC via kernel; common debian pattern)
		c.EnsureIPv6(interfaces.MethodManual)
		c.IPv6.SetOption("accept_ra", "1")
	}

	if err := interfaces.ValidateConnection(c); err != nil {
		return nil, err
	}
	return c, nil
}

// ---------- Activate / Deactivate ----------

func (m Model) actCandidates(up bool) []string {
	seen := map[string]bool{}
	var names []string
	for _, c := range m.conns {
		if c.Name == "lo" {
			continue
		}
		isUp := netdev.IsUp(c.Name)
		if up && !isUp {
			names = append(names, c.Name)
			seen[c.Name] = true
		}
		if !up && isUp {
			names = append(names, c.Name)
			seen[c.Name] = true
		}
	}
	// Also include devices not in config for activate? Only configured ones for ifup.
	for _, d := range m.devices {
		if d.IsLoop || seen[d.Name] {
			continue
		}
		// Offer system devices that have a connection or for deactivate if up
		if !up && netdev.IsUp(d.Name) {
			// only if configured
			continue
		}
	}
	return names
}

func (m Model) updateActList(msg tea.KeyMsg, activate bool) (tea.Model, tea.Cmd) {
	names := m.actCandidates(activate)
	switch msg.String() {
	case "esc", "q":
		m.screen = screenMenu
		m.status = ""
	case "up", "k":
		if m.actIdx > 0 {
			m.actIdx--
		}
	case "down", "j":
		if m.actIdx < len(names)-1 {
			m.actIdx++
		}
	case "enter", " ":
		if len(names) == 0 {
			m.status = "No interfaces available"
			return m, nil
		}
		m.confirmN = names[m.actIdx]
		if activate {
			m.confirm = confirmActivate
		} else {
			m.confirm = confirmDeactivate
		}
		m.screen = screenConfirm
	}
	return m, nil
}

func (m Model) viewActList(activate bool) string {
	title := "Activate a connection"
	if !activate {
		title = "Deactivate a connection"
	}
	names := m.actCandidates(activate)
	var b strings.Builder
	b.WriteString(sectionStyle.Render(title) + "\n\n")
	if len(names) == 0 {
		b.WriteString(subtleStyle.Render("  (No interfaces available)") + "\n")
		return b.String()
	}
	for i, n := range names {
		cursor := "  "
		style := itemStyle
		if i == m.actIdx {
			cursor = "> "
			style = selectedStyle
		}
		state := "DOWN"
		if netdev.IsUp(n) {
			state = "UP"
		}
		addrs := ""
		for _, d := range m.devices {
			if d.Name == n {
				addrs = strings.Join(d.Addrs, " ")
				break
			}
		}
		line := fmt.Sprintf("%s%-12s  [%s]  %s", cursor, n, state, addrs)
		b.WriteString(style.Render(line) + "\n")
	}
	return b.String()
}

// ---------- Confirm / Message ----------

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "n", "esc", "q":
		m.backFromConfirm(false)
	case "y", "enter":
		m.backFromConfirm(true)
	}
	return m, nil
}

func (m *Model) backFromConfirm(yes bool) {
	action := m.confirm
	name := m.confirmN
	m.confirm = confirmNone
	if !yes {
		switch action {
		case confirmSave:
			m.screen = screenEditForm
		case confirmDelete:
			m.screen = screenEditList
		case confirmActivate:
			m.screen = screenActivate
		case confirmDeactivate:
			m.screen = screenDeactivate
		default:
			m.screen = screenMenu
		}
		return
	}

	switch action {
	case confirmDelete:
		if err := m.file.DeleteConnection(name); err != nil {
			m.showMsg("Delete failed", err.Error(), screenEditList)
			return
		}
		m.reload()
		m.showMsg("Deleted", "Removed connection "+name+"\nA backup was created.", screenEditList)
	case confirmSave:
		c, err := m.buildConnFromForm()
		if err != nil {
			m.status = err.Error()
			m.screen = screenEditForm
			return
		}
		if err := m.file.SaveConnection(c); err != nil {
			m.showMsg("Save failed", err.Error(), screenEditForm)
			return
		}
		m.reload()
		m.showMsg("Saved", fmt.Sprintf("Connection %s written to %s\nOriginal file was backed up.\nUse \"Activate a connection\" to apply.", c.Name, m.cfgPath), screenEditList)
	case confirmActivate:
		if err := netdev.IfUp(name); err != nil {
			m.showMsg("Activate failed", err.Error(), screenActivate)
			return
		}
		m.reload()
		m.showMsg("Activated", "Ran ifup "+name+".", screenActivate)
	case confirmDeactivate:
		if err := netdev.IfDown(name); err != nil {
			m.showMsg("Deactivate failed", err.Error(), screenDeactivate)
			return
		}
		m.reload()
		m.showMsg("Deactivated", "Ran ifdown "+name+".", screenDeactivate)
	}
}

func (m *Model) showMsg(title, body string, back screen) {
	m.msgTitle = title
	m.msgBody = body
	m.msgBack = back
	m.screen = screenMessage
}

func (m Model) updateMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc", "q", " ":
		m.screen = m.msgBack
		m.status = ""
	}
	return m, nil
}

func (m Model) viewConfirm() string {
	var text string
	switch m.confirm {
	case confirmDelete:
		text = fmt.Sprintf("Delete connection %q?\nRelated stanzas will be removed from %s.", m.confirmN, m.cfgPath)
	case confirmSave:
		text = fmt.Sprintf("Save to %s?\nA backup will be created first.", m.cfgPath)
	case confirmActivate:
		text = fmt.Sprintf("Run ifup %s?", m.confirmN)
	case confirmDeactivate:
		text = fmt.Sprintf("Run ifdown %s?\nThis may interrupt network connectivity.", m.confirmN)
	}
	return sectionStyle.Render("Confirm") + "\n\n" + itemStyle.Render(text) + "\n\n" +
		selectedStyle.Render("  [y] Yes    [n] No")
}

func (m Model) viewMessage() string {
	return sectionStyle.Render(m.msgTitle) + "\n\n" + itemStyle.Render(m.msgBody)
}
