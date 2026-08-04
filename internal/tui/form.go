package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/debian-network-tui/debian-network-tui/internal/interfaces"
	"github.com/debian-network-tui/debian-network-tui/internal/netdev"
)

// Form focus indices (shared layout; unused fields skipped in navigation via visibleFields).
const (
	fName = iota
	fAuto
	fHotplug
	fVLANParent
	fVLANID
	fBondSlaves
	fBondMode
	fBondMiimon
	fBondLacp
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

// Text input slots:
// 0 name, 1 vlan parent, 2 vlan id, 3 bond slaves, 4 bond miimon, 5 bond lacp,
// 6 v4 addr, 7 netmask, 8 gateway, 9 dns, 10 v6 addr, 11 v6 gateway
const (
	inName = iota
	inVLANParent
	inVLANID
	inBondSlaves
	inBondMiimon
	inBondLacp
	inAddress
	inNetmask
	inGateway
	inDNS
	inV6Address
	inV6Gateway
	inCount
)

func (m *Model) visibleFields() []int {
	fields := []int{fName, fAuto, fHotplug}
	switch m.editType {
	case interfaces.TypeVLAN:
		fields = append(fields, fVLANParent, fVLANID)
	case interfaces.TypeBond:
		fields = append(fields, fBondSlaves, fBondMode, fBondMiimon, fBondLacp)
	}
	fields = append(fields,
		fIPv4Method, fAddress, fNetmask, fGateway, fDNS,
		fIPv6Method, fV6Address, fV6Gateway,
	)
	return fields
}

func (m *Model) focusIndex() int {
	vis := m.visibleFields()
	for i, f := range vis {
		if f == m.formFocus {
			return i
		}
	}
	return 0
}

func (m *Model) setFocusByVisible(delta int) {
	vis := m.visibleFields()
	if len(vis) == 0 {
		return
	}
	idx := m.focusIndex()
	idx = (idx + delta + len(vis)) % len(vis)
	m.formFocus = vis[idx]
	m.syncInputFocus()
	m.maybeRefreshVLANName()
}

func (m *Model) startEditForm(c *interfaces.Connection, isNew bool, ctype interfaces.ConnType) {
	m.editConn = c
	m.editNew = isNew
	m.editType = ctype
	m.autoOn = c.Auto
	m.hotplugOn = c.AllowHotplug

	m.ipv4Method = 3 // disabled
	if c.IPv4 != nil {
		switch c.IPv4.Method {
		case interfaces.MethodDHCP:
			m.ipv4Method = 0
		case interfaces.MethodStatic:
			m.ipv4Method = 1
		case interfaces.MethodManual:
			m.ipv4Method = 2
		default:
			m.ipv4Method = 0
		}
	} else if ctype == interfaces.TypeBond {
		m.ipv4Method = 2 // manual default for bond
	} else if ctype == interfaces.TypeEthernet || ctype == interfaces.TypeVLAN {
		if isNew {
			m.ipv4Method = 0 // dhcp
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

	mode := c.BondMode()
	m.bondModeIdx = 0
	for i, bm := range bondModes {
		if bm == mode {
			m.bondModeIdx = i
			break
		}
	}

	vals := make([]string, inCount)
	vals[inName] = c.Name
	vals[inVLANParent] = c.VLANParent()
	vals[inVLANID] = c.VLANID()
	vals[inBondSlaves] = strings.Join(c.BondSlaves(), " ")
	vals[inBondMiimon] = "100"
	vals[inBondLacp] = "fast"
	if c.IPv4 != nil {
		if v := c.IPv4.GetOption("bond-miimon"); v != "" {
			vals[inBondMiimon] = v
		}
		if v := c.IPv4.GetOption("bond-lacp-rate"); v != "" {
			vals[inBondLacp] = v
		}
		vals[inAddress] = c.IPv4.GetOption("address")
		vals[inNetmask] = c.IPv4.GetOption("netmask")
		vals[inGateway] = c.IPv4.GetOption("gateway")
		vals[inDNS] = c.IPv4.GetOption("dns-nameservers")
	}
	if c.IPv6 != nil {
		vals[inV6Address] = c.IPv6.GetOption("address")
		vals[inV6Gateway] = c.IPv6.GetOption("gateway")
	}

	placeholders := [inCount]string{
		"eth0 / ens18 / bond0",
		"parent: eth0 or bond0",
		"VLAN ID 1-4094",
		"slaves: eth0 eth1",
		"miimon ms",
		"lacp-rate: fast|slow",
		"IPv4 address",
		"netmask e.g. 255.255.255.0",
		"default gateway",
		"DNS space-separated",
		"IPv6 address/prefix",
		"IPv6 gateway",
	}
	widths := [inCount]int{24, 24, 10, 40, 10, 12, 24, 24, 24, 40, 40, 40}

	m.inputs = make([]textinput.Model, inCount)
	for i := 0; i < inCount; i++ {
		ti := textinput.New()
		ti.Placeholder = placeholders[i]
		ti.SetValue(vals[i])
		ti.CharLimit = 128
		ti.Width = widths[i]
		m.inputs[i] = ti
	}

	m.formFocus = fName
	if ctype == interfaces.TypeVLAN {
		m.formFocus = fVLANParent
	}
	m.syncInputFocus()
	m.maybeRefreshVLANName()
	m.screen = screenEditForm
	m.status = ""
}

func inputIndex(focus int) int {
	switch focus {
	case fName:
		return inName
	case fVLANParent:
		return inVLANParent
	case fVLANID:
		return inVLANID
	case fBondSlaves:
		return inBondSlaves
	case fBondMiimon:
		return inBondMiimon
	case fBondLacp:
		return inBondLacp
	case fAddress:
		return inAddress
	case fNetmask:
		return inNetmask
	case fGateway:
		return inGateway
	case fDNS:
		return inDNS
	case fV6Address:
		return inV6Address
	case fV6Gateway:
		return inV6Gateway
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

func (m *Model) maybeRefreshVLANName() {
	if m.editType != interfaces.TypeVLAN {
		return
	}
	parent := strings.TrimSpace(m.inputs[inVLANParent].Value())
	id := strings.TrimSpace(m.inputs[inVLANID].Value())
	if parent != "" && id != "" {
		m.inputs[inName].SetValue(parent + "." + id)
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
		m.setFocusByVisible(1)
		return m, nil
	case "shift+tab", "up":
		m.setFocusByVisible(-1)
		return m, nil
	case "left", "right", " ", "enter":
		if !m.isToggleFocus() {
			break
		}
		m.toggleFocused(msg.String() == "left")
		return m, nil
	}

	if idx := inputIndex(m.formFocus); idx >= 0 {
		var cmd tea.Cmd
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		if m.editType == interfaces.TypeVLAN && (idx == inVLANParent || idx == inVLANID) {
			m.maybeRefreshVLANName()
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) isToggleFocus() bool {
	switch m.formFocus {
	case fAuto, fHotplug, fIPv4Method, fIPv6Method, fBondMode:
		return true
	default:
		return false
	}
}

func (m *Model) toggleFocused(left bool) {
	switch m.formFocus {
	case fAuto:
		m.autoOn = !m.autoOn
	case fHotplug:
		m.hotplugOn = !m.hotplugOn
	case fIPv4Method:
		if left {
			m.ipv4Method = (m.ipv4Method - 1 + len(ipv4Methods)) % len(ipv4Methods)
		} else {
			m.ipv4Method = (m.ipv4Method + 1) % len(ipv4Methods)
		}
	case fIPv6Method:
		if left {
			m.ipv6Method = (m.ipv6Method - 1 + len(ipv6Methods)) % len(ipv6Methods)
		} else {
			m.ipv6Method = (m.ipv6Method + 1) % len(ipv6Methods)
		}
	case fBondMode:
		if left {
			m.bondModeIdx = (m.bondModeIdx - 1 + len(bondModes)) % len(bondModes)
		} else {
			m.bondModeIdx = (m.bondModeIdx + 1) % len(bondModes)
		}
	}
}

func (m Model) viewEditForm() string {
	var b strings.Builder
	title := "Edit " + string(m.editType)
	if m.editNew {
		title = "Add " + string(m.editType)
	}
	b.WriteString(sectionStyle.Render(title) + "\n\n")
	b.WriteString(subtleStyle.Render("  System NICs: "+m.deviceNamesHint()) + "\n\n")

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

	b.WriteString(row(fName, "Device", m.inputs[inName].View()))
	b.WriteString(row(fAuto, "Auto start", boolStr(m.autoOn)))
	b.WriteString(row(fHotplug, "Hotplug", boolStr(m.hotplugOn)))

	switch m.editType {
	case interfaces.TypeVLAN:
		b.WriteString("\n")
		b.WriteString(subtleStyle.Render("  —— VLAN (parent may be bondX) ——") + "\n")
		b.WriteString(row(fVLANParent, "Parent", m.inputs[inVLANParent].View()))
		b.WriteString(row(fVLANID, "VLAN ID", m.inputs[inVLANID].View()))
	case interfaces.TypeBond:
		b.WriteString("\n")
		b.WriteString(subtleStyle.Render("  —— Bond ——") + "\n")
		b.WriteString(row(fBondSlaves, "Slaves", m.inputs[inBondSlaves].View()))
		b.WriteString(row(fBondMode, "Mode", "< "+bondModes[m.bondModeIdx]+" >"))
		b.WriteString(row(fBondMiimon, "Miimon", m.inputs[inBondMiimon].View()))
		b.WriteString(row(fBondLacp, "LACP rate", m.inputs[inBondLacp].View()))
	}

	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  —— IPv4 ——") + "\n")
	b.WriteString(row(fIPv4Method, "Method", "< "+ipv4Methods[m.ipv4Method]+" >"))
	b.WriteString(row(fAddress, "Address", m.inputs[inAddress].View()))
	b.WriteString(row(fNetmask, "Netmask", m.inputs[inNetmask].View()))
	b.WriteString(row(fGateway, "Gateway", m.inputs[inGateway].View()))
	b.WriteString(row(fDNS, "DNS", m.inputs[inDNS].View()))
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  —— IPv6 ——") + "\n")
	b.WriteString(row(fIPv6Method, "Method", "< "+ipv6Methods[m.ipv6Method]+" >"))
	b.WriteString(row(fV6Address, "Address", m.inputs[inV6Address].View()))
	b.WriteString(row(fV6Gateway, "Gateway", m.inputs[inV6Gateway].View()))
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  Ctrl+S to save (backup created automatically)") + "\n")
	return b.String()
}

func (m *Model) buildConnFromForm() (*interfaces.Connection, error) {
	m.maybeRefreshVLANName()
	name := strings.TrimSpace(m.inputs[inName].Value())
	if name == "" {
		return nil, fmt.Errorf("interface name is required")
	}

	c := &interfaces.Connection{
		Name:         name,
		Auto:         m.autoOn,
		AllowHotplug: m.hotplugOn,
	}

	switch m.ipv4Method {
	case 0:
		c.EnsureIPv4(interfaces.MethodDHCP)
	case 1:
		c.EnsureIPv4(interfaces.MethodStatic)
		c.IPv4.SetOption("address", strings.TrimSpace(m.inputs[inAddress].Value()))
		c.IPv4.SetOption("netmask", strings.TrimSpace(m.inputs[inNetmask].Value()))
		c.IPv4.SetOption("gateway", strings.TrimSpace(m.inputs[inGateway].Value()))
		c.IPv4.SetOption("dns-nameservers", strings.TrimSpace(m.inputs[inDNS].Value()))
	case 2:
		c.EnsureIPv4(interfaces.MethodManual)
	case 3:
		c.ClearIPv4()
	}

	switch m.ipv6Method {
	case 0:
		c.ClearIPv6()
	case 1:
		c.EnsureIPv6(interfaces.MethodDHCP)
	case 2:
		c.EnsureIPv6(interfaces.MethodStatic)
		c.IPv6.SetOption("address", strings.TrimSpace(m.inputs[inV6Address].Value()))
		c.IPv6.SetOption("gateway", strings.TrimSpace(m.inputs[inV6Gateway].Value()))
	case 3:
		c.EnsureIPv6(interfaces.MethodManual)
		c.IPv6.SetOption("accept_ra", "1")
	}

	// Ensure a primary inet stanza exists so type-specific options can be stored.
	if c.IPv4 == nil && c.IPv6 == nil {
		c.EnsureIPv4(interfaces.MethodManual)
	}
	primary := c.IPv4
	if primary == nil {
		primary = c.IPv6
	}

	switch m.editType {
	case interfaces.TypeVLAN:
		parent := strings.TrimSpace(m.inputs[inVLANParent].Value())
		id := strings.TrimSpace(m.inputs[inVLANID].Value())
		if parent == "" || id == "" {
			return nil, fmt.Errorf("VLAN parent and ID are required")
		}
		if _, err := strconv.Atoi(id); err != nil {
			return nil, fmt.Errorf("VLAN ID must be a number")
		}
		if !strings.Contains(name, ".") {
			name = parent + "." + id
			c.Name = name
			if c.IPv4 != nil {
				c.IPv4.Name = name
			}
			if c.IPv6 != nil {
				c.IPv6.Name = name
			}
		}
		primary.SetOption("vlan-raw-device", parent)
		// vlan_id helps ifupdown-vlan; dotted names usually work without it
		primary.SetOption("vlan_id", id)
	case interfaces.TypeBond:
		slaves := strings.Fields(strings.TrimSpace(m.inputs[inBondSlaves].Value()))
		if len(slaves) == 0 {
			return nil, fmt.Errorf("bond slaves are required (e.g. eth0 eth1)")
		}
		primary.SetOption("bond-slaves", strings.Join(slaves, " "))
		primary.SetOption("bond-mode", bondModes[m.bondModeIdx])
		miimon := strings.TrimSpace(m.inputs[inBondMiimon].Value())
		if miimon == "" {
			miimon = "100"
		}
		primary.SetOption("bond-miimon", miimon)
		if bondModes[m.bondModeIdx] == "802.3ad" {
			lacp := strings.TrimSpace(m.inputs[inBondLacp].Value())
			if lacp == "" {
				lacp = "fast"
			}
			primary.SetOption("bond-lacp-rate", lacp)
		} else {
			primary.SetOption("bond-lacp-rate", "")
		}
	}

	if err := interfaces.ValidateConnection(c); err != nil {
		return nil, err
	}
	return c, nil
}

// ---------- Activate / Deactivate ----------

func (m Model) actCandidates(wantUp bool) []string {
	seen := map[string]bool{}
	var names []string
	for _, c := range m.conns {
		if c.Name == "lo" || !c.Configured() {
			continue
		}
		isUp := netdev.IsUp(c.Name)
		if wantUp && !isUp {
			names = append(names, c.Name)
			seen[c.Name] = true
		}
		if !wantUp && isUp {
			names = append(names, c.Name)
			seen[c.Name] = true
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
		line := fmt.Sprintf("%s%-14s  [%s]  %s", cursor, n, state, addrs)
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
		case confirmRestartNetworking:
			m.screen = screenMenu
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
		extra := ""
		if c.Type() == interfaces.TypeBond {
			extra = "\nSlave interfaces were set to manual + bond-master."
		}
		m.reload()
		m.showMsg("Saved", fmt.Sprintf("Connection %s written to %s\nOriginal file was backed up.%s\nUse \"Activate a connection\" or \"Restart networking\" to apply.", c.Name, m.cfgPath, extra), screenEditList)
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
	case confirmRestartNetworking:
		if err := netdev.ReloadNetworking(); err != nil {
			m.showMsg("Restart failed", err.Error(), screenMenu)
			return
		}
		m.reload()
		m.showMsg("Networking restarted", "Networking service was restarted.\nSSH sessions may drop briefly.", screenMenu)
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
	case confirmRestartNetworking:
		text = "Restart networking service?\n(systemctl restart networking)\nThis may interrupt SSH / network connectivity."
	}
	return sectionStyle.Render("Confirm") + "\n\n" + itemStyle.Render(text) + "\n\n" +
		selectedStyle.Render("  [y] Yes    [n] No")
}

func (m Model) viewMessage() string {
	return sectionStyle.Render(m.msgTitle) + "\n\n" + itemStyle.Render(m.msgBody)
}
