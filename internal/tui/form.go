package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/debian-network-tui/debian-network-tui/internal/aptsources"
	"github.com/debian-network-tui/debian-network-tui/internal/bootstrap"
	"github.com/debian-network-tui/debian-network-tui/internal/interfaces"
	"github.com/debian-network-tui/debian-network-tui/internal/netdev"
	"github.com/debian-network-tui/debian-network-tui/internal/packages"
	"github.com/debian-network-tui/debian-network-tui/internal/resolvconf"
	"github.com/debian-network-tui/debian-network-tui/internal/sshsetup"
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
	fIPv6Method
	fV6Address
	fV6Gateway
	fCount
)

// Text input slots (VLAN parent / bond slaves use pickers, not text fields):
// 0 name, 1 unused, 2 vlan id, 3 unused, 4 bond miimon, 5 bond lacp,
// 6 v4 addr, 7 netmask, 8 gateway, 9 unused, 10 v6 addr, 11 v6 gateway
const (
	inName = iota
	inUnusedParent
	inVLANID
	inUnusedSlaves
	inBondMiimon
	inBondLacp
	inAddress
	inNetmask
	inGateway
	inUnusedDNS
	inV6Address
	inV6Gateway
	inCount
)

func (m *Model) visibleFields() []int {
	fields := []int{fAuto, fHotplug}
	switch m.editType {
	case interfaces.TypeVLAN:
		fields = append([]int{fVLANParent, fVLANID}, fields...)
	case interfaces.TypeBond:
		fields = append([]int{fName}, fields...)
		fields = append(fields, fBondSlaves, fBondMode, fBondMiimon, fBondLacp)
	default:
		fields = append([]int{fName}, fields...)
	}
	fields = append(fields,
		fIPv4Method, fAddress, fNetmask, fGateway,
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
	if m.formFocus == fBondSlaves {
		m.refreshBondCandidates()
		if m.bondSlaveIdx >= len(m.bondCandidates) {
			m.bondSlaveIdx = 0
		}
	}
	if m.formFocus == fVLANParent {
		m.refreshVLANParentCandidates()
		if m.vlanParentIdx >= len(m.vlanParentCandidates) {
			m.vlanParentIdx = 0
		}
	}
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
	}
	// New ethernet/vlan keep ipv4Method=disabled (3).

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
	vals[inVLANID] = c.VLANID()
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
	}
	if c.IPv6 != nil {
		vals[inV6Address] = c.IPv6.GetOption("address")
		vals[inV6Gateway] = c.IPv6.GetOption("gateway")
	}

	placeholders := [inCount]string{
		"auto: parent.vlanid",
		"",
		"e.g. 100",
		"",
		"miimon ms",
		"lacp-rate: fast|slow",
		"IPv4 address",
		"netmask e.g. 255.255.255.0",
		"default gateway",
		"",
		"IPv6 address/prefix",
		"IPv6 gateway",
	}
	widths := [inCount]int{24, 8, 16, 8, 10, 12, 24, 24, 24, 40, 40, 40}

	m.inputs = make([]textinput.Model, inCount)
	for i := 0; i < inCount; i++ {
		ti := textinput.New()
		ti.Placeholder = placeholders[i]
		ti.SetValue(vals[i])
		ti.CharLimit = 128
		if i == inVLANID {
			ti.CharLimit = 4
		}
		ti.Width = widths[i]
		m.inputs[i] = ti
	}

	m.initBondSlavePicker(c.BondSlaves())
	m.initVLANParentPicker(c.VLANParent())

	m.formFocus = fName
	if ctype == interfaces.TypeVLAN {
		m.formFocus = fVLANParent
	}
	if ctype == interfaces.TypeBond {
		m.formFocus = fBondSlaves
	}
	m.syncInputFocus()
	m.maybeRefreshVLANName()
	m.screen = screenEditForm
	m.status = ""
}

// initBondSlavePicker builds a checklist of UP NICs (plus already-selected slaves).
func (m *Model) initBondSlavePicker(preselected []string) {
	m.bondSelected = map[string]bool{}
	for _, s := range preselected {
		s = strings.TrimSpace(s)
		if s != "" {
			m.bondSelected[s] = true
		}
	}
	m.bondSlaveIdx = 0
	m.refreshBondCandidates()
}

func (m *Model) refreshBondCandidates() {
	bondName := ""
	if len(m.inputs) > inName {
		bondName = strings.TrimSpace(m.inputs[inName].Value())
	}
	seen := map[string]bool{}
	var cands []string
	for _, d := range m.devices {
		if d.IsLoop || d.Name == "" || d.Name == bondName {
			continue
		}
		switch d.Kind {
		case "bond", "vlan", "bridge":
			continue
		}
		up := strings.EqualFold(d.State, "up") || netdev.IsUp(d.Name)
		if !up && !m.bondSelected[d.Name] {
			continue
		}
		cands = append(cands, d.Name)
		seen[d.Name] = true
	}
	for name, sel := range m.bondSelected {
		if sel && !seen[name] && name != bondName {
			cands = append(cands, name)
		}
	}
	sort.Strings(cands)
	m.bondCandidates = cands
	if len(cands) == 0 {
		m.bondSlaveIdx = 0
		return
	}
	if m.bondSlaveIdx >= len(cands) {
		m.bondSlaveIdx = len(cands) - 1
	}
}

func (m *Model) selectedBondSlaves() []string {
	var out []string
	for _, name := range m.bondCandidates {
		if m.bondSelected[name] {
			out = append(out, name)
		}
	}
	// Include selected that somehow dropped off the candidate list.
	for name, sel := range m.bondSelected {
		if !sel {
			continue
		}
		found := false
		for _, n := range out {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Model) toggleBondSlaveAt(idx int) {
	if idx < 0 || idx >= len(m.bondCandidates) {
		return
	}
	name := m.bondCandidates[idx]
	m.bondSelected[name] = !m.bondSelected[name]
}

// initVLANParentPicker builds a single-select list of UP parents (ethernet or bond).
func (m *Model) initVLANParentPicker(preselected string) {
	m.vlanParentSelected = strings.TrimSpace(preselected)
	m.vlanParentIdx = 0
	m.refreshVLANParentCandidates()
	if m.vlanParentSelected != "" {
		for i, name := range m.vlanParentCandidates {
			if name == m.vlanParentSelected {
				m.vlanParentIdx = i
				break
			}
		}
	}
}

func (m *Model) refreshVLANParentCandidates() {
	seen := map[string]bool{}
	var cands []string
	for _, d := range m.devices {
		if d.IsLoop || d.Name == "" {
			continue
		}
		// Parents may be ethernet or bond; skip existing VLANs/bridges.
		switch d.Kind {
		case "vlan", "bridge":
			continue
		}
		up := strings.EqualFold(d.State, "up") || netdev.IsUp(d.Name)
		if !up && d.Name != m.vlanParentSelected {
			continue
		}
		cands = append(cands, d.Name)
		seen[d.Name] = true
	}
	if m.vlanParentSelected != "" && !seen[m.vlanParentSelected] {
		cands = append(cands, m.vlanParentSelected)
	}
	sort.Strings(cands)
	m.vlanParentCandidates = cands
	if len(cands) == 0 {
		m.vlanParentIdx = 0
		return
	}
	if m.vlanParentIdx >= len(cands) {
		m.vlanParentIdx = len(cands) - 1
	}
}

func (m *Model) selectVLANParentAt(idx int) {
	if idx < 0 || idx >= len(m.vlanParentCandidates) {
		return
	}
	m.vlanParentSelected = m.vlanParentCandidates[idx]
	m.maybeRefreshVLANName()
}

func inputIndex(focus int) int {
	switch focus {
	case fName:
		return inName
	case fVLANID:
		return inVLANID
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
	parent := strings.TrimSpace(m.vlanParentSelected)
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
	case "tab":
		m.setFocusByVisible(1)
		return m, nil
	case "shift+tab":
		m.setFocusByVisible(-1)
		return m, nil
	case "down", "j":
		if m.formFocus == fBondSlaves {
			if len(m.bondCandidates) == 0 {
				m.setFocusByVisible(1)
				return m, nil
			}
			if m.bondSlaveIdx < len(m.bondCandidates)-1 {
				m.bondSlaveIdx++
				return m, nil
			}
			m.setFocusByVisible(1)
			return m, nil
		}
		if m.formFocus == fVLANParent {
			if len(m.vlanParentCandidates) == 0 {
				m.setFocusByVisible(1)
				return m, nil
			}
			if m.vlanParentIdx < len(m.vlanParentCandidates)-1 {
				m.vlanParentIdx++
				return m, nil
			}
			m.setFocusByVisible(1)
			return m, nil
		}
		m.setFocusByVisible(1)
		return m, nil
	case "up", "k":
		if m.formFocus == fBondSlaves {
			if m.bondSlaveIdx > 0 {
				m.bondSlaveIdx--
				return m, nil
			}
			m.setFocusByVisible(-1)
			return m, nil
		}
		if m.formFocus == fVLANParent {
			if m.vlanParentIdx > 0 {
				m.vlanParentIdx--
				return m, nil
			}
			m.setFocusByVisible(-1)
			return m, nil
		}
		m.setFocusByVisible(-1)
		return m, nil
	case " ", "enter":
		if m.formFocus == fBondSlaves {
			m.toggleBondSlaveAt(m.bondSlaveIdx)
			return m, nil
		}
		if m.formFocus == fVLANParent {
			m.selectVLANParentAt(m.vlanParentIdx)
			return m, nil
		}
		if m.isToggleFocus() {
			m.toggleFocused(false)
			return m, nil
		}
	case "left", "right":
		if m.isToggleFocus() {
			m.toggleFocused(msg.String() == "left")
			return m, nil
		}
	}

	if idx := inputIndex(m.formFocus); idx >= 0 {
		var cmd tea.Cmd
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		if m.editType == interfaces.TypeVLAN && idx == inVLANID {
			m.maybeRefreshVLANName()
		}
		if m.editType == interfaces.TypeBond && idx == inName {
			m.refreshBondCandidates()
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

	switch m.editType {
	case interfaces.TypeVLAN:
		devName := strings.TrimSpace(m.inputs[inName].Value())
		if devName == "" {
			devName = "(select parent + VLAN ID)"
		}
		b.WriteString(subtleStyle.Render(fmt.Sprintf("  Device         %s", devName)) + "\n")
		b.WriteString(row(fAuto, "Auto start", boolStr(m.autoOn)))
		b.WriteString(row(fHotplug, "Hotplug", boolStr(m.hotplugOn)))
		b.WriteString("\n")
		b.WriteString(subtleStyle.Render("  —— VLAN parent (UP NICs/bonds — Space/Enter to select) ——") + "\n")
		if len(m.vlanParentCandidates) == 0 {
			style := itemStyle
			if m.formFocus == fVLANParent {
				style = selectedStyle
			}
			b.WriteString(style.Render("  (No UP interfaces found)") + "\n")
		} else {
			for i, name := range m.vlanParentCandidates {
				cursor := "  "
				style := itemStyle
				if m.formFocus == fVLANParent && i == m.vlanParentIdx {
					cursor = "> "
					style = selectedStyle
				}
				mark := "( )"
				if name == m.vlanParentSelected {
					mark = "(*)"
				}
				state := "?"
				kind := ""
				for _, d := range m.devices {
					if d.Name == name {
						state = strings.ToUpper(d.State)
						if state == "" {
							if netdev.IsUp(name) {
								state = "UP"
							} else {
								state = "DOWN"
							}
						}
						kind = d.Kind
						break
					}
				}
				line := fmt.Sprintf("%s%s %-12s %-6s %s", cursor, mark, name, state, kind)
				b.WriteString(style.Render(line) + "\n")
			}
		}
		sel := m.vlanParentSelected
		if sel == "" {
			sel = "(none)"
		}
		b.WriteString(subtleStyle.Render("  Selected parent: "+sel) + "\n")
		b.WriteString(row(fVLANID, "VLAN ID", m.inputs[inVLANID].View()))
		b.WriteString(subtleStyle.Render("  Valid VLAN ID range: 2-4094") + "\n")
	case interfaces.TypeBond:
		b.WriteString(row(fName, "Device", m.inputs[inName].View()))
		b.WriteString(row(fAuto, "Auto start", boolStr(m.autoOn)))
		b.WriteString(row(fHotplug, "Hotplug", boolStr(m.hotplugOn)))
		b.WriteString("\n")
		b.WriteString(subtleStyle.Render("  —— Bond slaves (UP NICs — Space/Enter to toggle) ——") + "\n")
		if len(m.bondCandidates) == 0 {
			style := itemStyle
			if m.formFocus == fBondSlaves {
				style = selectedStyle
			}
			b.WriteString(style.Render("  (No UP interfaces found)") + "\n")
		} else {
			for i, name := range m.bondCandidates {
				cursor := "  "
				style := itemStyle
				if m.formFocus == fBondSlaves && i == m.bondSlaveIdx {
					cursor = "> "
					style = selectedStyle
				}
				mark := "[ ]"
				if m.bondSelected[name] {
					mark = "[x]"
				}
				state := "?"
				mac := ""
				for _, d := range m.devices {
					if d.Name == name {
						state = strings.ToUpper(d.State)
						if state == "" {
							if netdev.IsUp(name) {
								state = "UP"
							} else {
								state = "DOWN"
							}
						}
						mac = d.MAC
						break
					}
				}
				line := fmt.Sprintf("%s%s %-12s %-6s %s", cursor, mark, name, state, mac)
				b.WriteString(style.Render(line) + "\n")
			}
		}
		sel := m.selectedBondSlaves()
		b.WriteString(subtleStyle.Render("  Selected: "+strings.Join(sel, " ")) + "\n")
		b.WriteString(row(fBondMode, "Mode", "< "+bondModes[m.bondModeIdx]+" >"))
		b.WriteString(row(fBondMiimon, "Miimon", m.inputs[inBondMiimon].View()))
		b.WriteString(row(fBondLacp, "LACP rate", m.inputs[inBondLacp].View()))
	default:
		b.WriteString(row(fName, "Device", m.inputs[inName].View()))
		b.WriteString(row(fAuto, "Auto start", boolStr(m.autoOn)))
		b.WriteString(row(fHotplug, "Hotplug", boolStr(m.hotplugOn)))
	}

	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  —— IPv4 ——") + "\n")
	b.WriteString(row(fIPv4Method, "Method", "< "+ipv4Methods[m.ipv4Method]+" >"))
	b.WriteString(row(fAddress, "Address", m.inputs[inAddress].View()))
	b.WriteString(row(fNetmask, "Netmask", m.inputs[inNetmask].View()))
	b.WriteString(row(fGateway, "Gateway", m.inputs[inGateway].View()))
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
		parent := strings.TrimSpace(m.vlanParentSelected)
		id := strings.TrimSpace(m.inputs[inVLANID].Value())
		if parent == "" {
			return nil, fmt.Errorf("select a VLAN parent interface")
		}
		if id == "" {
			return nil, fmt.Errorf("VLAN ID is required")
		}
		vid, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("VLAN ID must be a number")
		}
		if vid < 2 || vid > 4094 {
			return nil, fmt.Errorf("VLAN ID must be 2-4094")
		}
		name = parent + "." + id
		c.Name = name
		if c.IPv4 != nil {
			c.IPv4.Name = name
		}
		if c.IPv6 != nil {
			c.IPv6.Name = name
		}
		primary.SetOption("vlan-raw-device", parent)
		primary.SetOption("vlan_id", id)
	case interfaces.TypeBond:
		slaves := m.selectedBondSlaves()
		if len(slaves) == 0 {
			return nil, fmt.Errorf("select at least one UP slave interface")
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
		return m, m.backFromConfirm(false)
	case "y", "enter":
		return m, m.backFromConfirm(true)
	}
	return m, nil
}

func (m *Model) backFromConfirm(yes bool) tea.Cmd {
	action := m.confirm
	name := m.confirmN
	m.confirm = confirmNone
	defer m.touch()
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
		case confirmClearAll:
			m.screen = screenMenu
		case confirmInstallDebs:
			m.screen = screenMenu
		case confirmClearAptSources:
			m.screen = screenMenu
		case confirmApplyAptSources:
			m.screen = screenMenu
		case confirmSaveDNS:
			m.screen = screenDNS
		case confirmApplyDNSFile:
			m.screen = m.pendingDNSBack
		case confirmSetupSSH:
			m.screen = screenMenu
		case confirmOneShot:
			m.screen = screenMenu
			m.pendingBoot = nil
		default:
			m.screen = screenMenu
		}
		return nil
	}

	switch action {
	case confirmDelete:
		if err := m.file.DeleteConnection(name); err != nil {
			m.showMsg("Delete failed", err.Error(), screenEditList)
			return nil
		}
		m.reload()
		m.showMsg("Deleted", "Removed connection "+name+"\nA backup was created.", screenEditList)
	case confirmSave:
		c, err := m.buildConnFromForm()
		if err != nil {
			m.status = err.Error()
			m.screen = screenEditForm
			return nil
		}
		if err := m.file.SaveConnection(c); err != nil {
			m.showMsg("Save failed", err.Error(), screenEditForm)
			return nil
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
			return nil
		}
		m.reload()
		m.showMsg("Activated", "Ran ifup "+name+".", screenActivate)
	case confirmDeactivate:
		if err := netdev.IfDown(name); err != nil {
			m.showMsg("Deactivate failed", err.Error(), screenDeactivate)
			return nil
		}
		m.reload()
		m.showMsg("Deactivated", "Ran ifdown "+name+".", screenDeactivate)
	case confirmRestartNetworking:
		if err := netdev.ReloadNetworking(); err != nil {
			m.showMsg("Restart failed", err.Error(), screenMenu)
			return nil
		}
		m.reload()
		m.showMsg("Networking restarted", "Networking service was restarted.\nSSH sessions may drop briefly.", screenMenu)
	case confirmClearAll:
		if err := m.file.ClearAllConnections(); err != nil {
			m.showMsg("Clear failed", err.Error(), screenMenu)
			return nil
		}
		m.reload()
		m.showMsg("Cleared", fmt.Sprintf("All connections removed from %s\n(and interfaces.d drop-ins).\nOnly loopback (lo) remains.\nBackups were created.\nUse \"Restart networking\" to apply.", m.cfgPath), screenMenu)
	case confirmInstallDebs:
		debs := append([]string{}, m.pendingDebs...)
		msg, err := packages.InstallLocalDebs(debs)
		m.pendingDebs = nil
		if err != nil {
			body := err.Error()
			if msg != "" {
				body = msg
			}
			m.showMsg("Install failed", body, screenMenu)
			return nil
		}
		if len(msg) > 1200 {
			msg = msg[len(msg)-1200:]
			msg = "...\n" + msg
		}
		m.showMsg("Install complete", "Installed local packages with apt:\n"+msg, screenMenu)
	case confirmClearAptSources:
		note, err := aptsources.Default().Clear()
		if err != nil {
			m.showMsg("Clear apt sources failed", err.Error()+"\n"+note, screenMenu)
			return nil
		}
		m.showMsg("Apt sources cleared", note+"\n\nRun apt-get update after applying new sources.", screenMenu)
	case confirmApplyAptSources:
		cfgs := append([]aptsources.LocalConfig{}, m.pendingAptCfgs...)
		m.pendingAptCfgs = nil
		note, err := aptsources.Default().Apply(cfgs)
		if err != nil {
			m.showMsg("Apply apt sources failed", err.Error()+"\n"+note, screenMenu)
			return nil
		}
		m.showMsg("Apt sources applied", note+"\n\nSuggested: apt-get update", screenMenu)
	case confirmSaveDNS:
		cfg, err := m.buildDNSFromForm()
		if err != nil {
			m.status = err.Error()
			m.screen = screenDNS
			return nil
		}
		if err := cfg.Save(); err != nil {
			m.showMsg("DNS save failed", err.Error(), screenDNS)
			return nil
		}
		extra := ""
		if m.dnsSymlink != "" {
			extra = "\nReplaced symlink with a regular file."
		}
		m.showMsg("DNS saved", fmt.Sprintf("Wrote %s\nBackup created.%s", cfg.Path, extra), screenMenu)
	case confirmApplyDNSFile:
		files := append([]string{}, m.pendingDNSFiles...)
		dest := m.pendingDNSDest
		dir := m.pendingDNSDir
		m.pendingDNSFiles = nil
		if dest == "" {
			dest = resolvconf.DefaultPath
		}
		if len(files) == 0 {
			m.showMsg("Apply DNS failed", "No local resolv.conf found.", screenMenu)
			return nil
		}
		src := files[0]
		bak, err := resolvconf.OverwriteFromFile(src, dest)
		if err != nil {
			m.showMsg("Apply DNS failed", err.Error(), screenMenu)
			return nil
		}
		extra := fmt.Sprintf("Source: %s\nDest: %s", src, dest)
		if bak != "" {
			extra += "\nBackup: " + bak
		}
		if len(files) > 1 {
			extra += fmt.Sprintf("\n(Other candidates in %s were ignored; used first match.)", dir)
		}
		m.showMsg("DNS overwritten", extra, screenMenu)
	case confirmSetupSSH:
		dir := m.pendingSSHDir
		res, err := sshsetup.Run(dir, packages.InstallLocalDebs)
		m.pendingSSHDebs = nil
		if err != nil {
			detail := err.Error()
			if res != nil && res.InstallDetail != "" {
				detail = res.InstallDetail + "\n\n" + detail
			}
			m.showMsg("SSH setup failed", detail, screenMenu)
			return nil
		}
		msg := fmt.Sprintf("Install: %s\nPubkey: %s\nKeys imported: %d new / %d total\nSSHD drop-in: %s\nPermitRootLogin: prohibit-password (key only)\nSSH service restarted.",
			res.InstallMethod, res.PubkeyFile, res.KeysAdded, res.KeysTotal, res.SSHDDropIn)
		if len(res.InstallDetail) > 400 {
			msg += "\n\napt output (truncated):\n..." + res.InstallDetail[len(res.InstallDetail)-400:]
		} else if res.InstallDetail != "" {
			msg += "\n\n" + res.InstallDetail
		}
		m.showMsg("SSH configured", msg, screenMenu)
	case confirmOneShot:
		if m.pendingBoot == nil {
			m.showMsg("One-shot setup failed", "No plan prepared.", screenMenu)
			return nil
		}
		m.bootLog = []string{
			"Starting one-shot setup…",
			"Order: DNS → clear apt → apply apt → ifenslave/vlan/net-tools → SSH",
			"",
		}
		m.bootScroll = 0
		m.bootDone = false
		m.bootErr = nil
		m.screen = screenBootLog
		return bootStepCmd(m.pendingBoot, bootstrap.StepDNS)
	}
	return nil
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
	case confirmClearAll:
		text = fmt.Sprintf("Clear ALL network connections in %s?\nAlso clears interfaces.d drop-ins.\nOnly loopback (lo) will remain.\nBackups will be created.\nTHIS CAN CUT REMOTE ACCESS.", m.cfgPath)
	case confirmInstallDebs:
		var names []string
		for _, p := range m.pendingDebs {
			names = append(names, filepath.Base(p))
		}
		text = fmt.Sprintf("Install local packages with apt from:\n%s\n\n%s\n\nRun: apt-get install -y <debs>",
			m.pendingDebDir, strings.Join(names, "\n"))
	case confirmClearAptSources:
		text = "Clear ALL apt source configuration?\n\n/etc/apt/sources.list will be emptied\n(with backup).\nAll .list / .sources in sources.list.d\nwill be backed up and removed.\n\nPackage installs may fail until new\nsources are applied."
	case confirmApplyAptSources:
		var lines []string
		for _, c := range m.pendingAptCfgs {
			dest := "/etc/apt/sources.list"
			if !c.IsPrimary {
				dest = "/etc/apt/sources.list.d/" + c.TargetRel
			}
			lines = append(lines, fmt.Sprintf("%s → %s", filepath.Base(c.Path), dest))
		}
		text = fmt.Sprintf("Apply apt sources from:\n%s\n\n%s\n\nExisting targets are backed up first.",
			m.pendingAptDir, strings.Join(lines, "\n"))
	case confirmSaveDNS:
		text = fmt.Sprintf("Save DNS settings to %s?\nA backup will be created first.", m.dnsPath)
		if m.dnsSymlink != "" {
			text += "\nSymlink will be replaced with a regular file."
		}
	case confirmApplyDNSFile:
		src := "(none)"
		if len(m.pendingDNSFiles) > 0 {
			src = m.pendingDNSFiles[0]
		}
		dest := m.pendingDNSDest
		if dest == "" {
			dest = resolvconf.DefaultPath
		}
		text = fmt.Sprintf("Overwrite DNS config?\n\n%s\n  → %s\n\nExisting file is backed up first.\nSymlink (if any) becomes a regular file.",
			src, dest)
	case confirmSetupSSH:
		debLine := "none (will use apt)"
		if len(m.pendingSSHDebs) > 0 {
			var names []string
			for _, p := range m.pendingSSHDebs {
				names = append(names, filepath.Base(p))
			}
			debLine = strings.Join(names, ", ")
		}
		text = fmt.Sprintf("Configure OpenSSH for root key login?\n\nDir: %s\nInstall via: %s\nLocal debs: %s\nPubkey file: %s\n\nWill set PermitRootLogin prohibit-password,\nimport key into /root/.ssh/authorized_keys,\nand restart ssh.",
			m.pendingSSHDir, m.pendingSSHMethod, debLine, m.pendingSSHPub)
	case confirmOneShot:
		summary := "(no plan)"
		if m.pendingBoot != nil {
			summary = strings.Join(m.pendingBoot.SummaryLines(), "\n")
		}
		text = "Run ONE-SHOT setup?\n\n" + summary +
			"\n\nThis will modify DNS, apt sources, install packages,\nand configure SSH. Backups are created where applicable.\nLive logs will be shown on the next screen."
	}
	return sectionStyle.Render("Confirm") + "\n\n" + itemStyle.Render(text) + "\n\n" +
		selectedStyle.Render("  [y] Yes    [n] No")
}

func (m Model) viewMessage() string {
	return sectionStyle.Render(m.msgTitle) + "\n\n" + itemStyle.Render(m.msgBody)
}
