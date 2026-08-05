package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/debian-network-tui/debian-network-tui/internal/aptsources"
	"github.com/debian-network-tui/debian-network-tui/internal/interfaces"
	"github.com/debian-network-tui/debian-network-tui/internal/netdev"
	"github.com/debian-network-tui/debian-network-tui/internal/packages"
	"github.com/debian-network-tui/debian-network-tui/internal/sshsetup"
)

type screen int

const (
	screenMenu screen = iota
	screenEditList
	screenAddType
	screenEditForm
	screenDNS
	screenActivate
	screenDeactivate
	screenConfirm
	screenMessage
)

type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmDelete
	confirmSave
	confirmActivate
	confirmDeactivate
	confirmRestartNetworking
	confirmClearAll
	confirmInstallDebs
	confirmClearAptSources
	confirmApplyAptSources
	confirmSaveDNS
	confirmApplyDNSFile
	confirmSetupSSH
)

// Model is the root Bubble Tea model.
type Model struct {
	cfgPath string
	file    *interfaces.File
	screen  screen
	width   int
	height  int

	menuIdx   int
	listIdx   int
	addTypeIdx int
	formFocus int
	actIdx    int

	conns    []*interfaces.Connection
	devices  []netdev.Device
	editConn *interfaces.Connection
	editNew  bool
	editType interfaces.ConnType
	inputs   []textinput.Model

	autoOn      bool
	hotplugOn   bool
	ipv4Method  int
	ipv6Method  int
	bondModeIdx int

	// Bond slave picker (checklist of UP / already-selected NICs).
	bondCandidates []string
	bondSelected   map[string]bool
	bondSlaveIdx   int

	// VLAN parent picker (single-select from UP NICs / bonds).
	vlanParentCandidates []string
	vlanParentSelected   string
	vlanParentIdx        int

	pendingDebs   []string
	pendingDebDir string

	pendingAptCfgs []aptsources.LocalConfig
	pendingAptDir  string

	pendingDNSFiles []string
	pendingDNSDir   string
	pendingDNSDest  string
	pendingDNSBack  screen

	pendingSSHDir   string
	pendingSSHDebs  []string
	pendingSSHPub   string
	pendingSSHMethod string // "local-deb" or "apt" preview

	dnsInputs  []textinput.Model
	dnsFocus   int
	dnsPath    string
	dnsSymlink string
	dnsWarn    string

	status   string
	errMsg   string
	confirm  confirmAction
	confirmN string
	msgTitle string
	msgBody  string
	msgBack  screen

	lastActivity time.Time
	idleTimeout  time.Duration
	idleTimedOut bool

	quit bool
}

var menuItems = []string{
	"Edit a connection",
	"Edit DNS (/etc/resolv.conf)",
	"Apply DNS from file (overwrite)",
	"Activate a connection",
	"Deactivate a connection",
	"Restart networking",
	"Clear all connections",
	"Install ifenslave/vlan (.deb)",
	"Clear apt sources",
	"Apply apt sources from file",
	"Configure SSH server (root key)",
	"Quit",
}

var addTypeItems = []string{
	"Ethernet",
	"VLAN (on ethernet or bond)",
	"Bond (bonding)",
}

var ipv4Methods = []string{"dhcp", "static", "manual", "disabled"}
var ipv6Methods = []string{"disabled", "dhcp", "static", "auto"}
var bondModes = []string{
	"802.3ad",
	"active-backup",
	"balance-rr",
	"balance-xor",
	"broadcast",
	"balance-tlb",
	"balance-alb",
}

func New(cfgPath string) Model {
	m := Model{
		cfgPath:     cfgPath,
		screen:      screenMenu,
		idleTimeout: idleTimeoutFromEnv(),
	}
	m.touch()
	m.reload()
	return m
}

func (m *Model) reload() {
	f, err := interfaces.Load(m.cfgPath)
	if err != nil {
		m.errMsg = err.Error()
		m.file = &interfaces.File{Path: m.cfgPath}
		m.conns = nil
	} else {
		m.file = f
		m.conns = f.Connections()
		m.errMsg = ""
	}
	devs, err := netdev.ListDevices()
	if err != nil {
		m.devices = nil
	} else {
		m.devices = devs
	}
	names := make([]string, 0, len(m.devices))
	for _, d := range m.devices {
		names = append(names, d.Name)
	}
	m.conns = interfaces.MergeWithDevices(m.conns, names)
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.idleTimeout > 0 {
		cmds = append(cmds, tickIdle())
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case idleTickMsg:
		if m.idleTimeout > 0 && time.Since(m.lastActivity) >= m.idleTimeout {
			m.idleTimedOut = true
			return m, tea.Quit
		}
		return m, tickIdle()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		m.touch()
		if m.quit {
			return m, tea.Quit
		}
		switch m.screen {
		case screenMenu:
			return m.updateMenu(msg)
		case screenEditList:
			return m.updateEditList(msg)
		case screenAddType:
			return m.updateAddType(msg)
		case screenEditForm:
			return m.updateEditForm(msg)
		case screenDNS:
			return m.updateDNSForm(msg)
		case screenActivate:
			return m.updateActList(msg, true)
		case screenDeactivate:
			return m.updateActList(msg, false)
		case screenConfirm:
			return m.updateConfirm(msg)
		case screenMessage:
			return m.updateMessage(msg)
		}
	}
	return m, nil
}

func (m Model) View() string {
	var body string
	switch m.screen {
	case screenMenu:
		body = m.viewMenu()
	case screenEditList:
		body = m.viewEditList()
	case screenAddType:
		body = m.viewAddType()
	case screenEditForm:
		body = m.viewEditForm()
	case screenDNS:
		body = m.viewDNSForm()
	case screenActivate:
		body = m.viewActList(true)
	case screenDeactivate:
		body = m.viewActList(false)
	case screenConfirm:
		body = m.viewConfirm()
	case screenMessage:
		body = m.viewMessage()
	default:
		body = "Unknown screen"
	}

	header := titleStyle.Render("debian-network-tui") + "  " +
		subtleStyle.Render("Manage /etc/network/interfaces · Debian 11–13")
	footer := m.viewFooter()
	status := ""
	if m.status != "" {
		status = statusStyle.Render(m.status)
	}
	if m.errMsg != "" && m.screen == screenMenu {
		status = errorStyle.Render(m.errMsg)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		body,
		"",
		status,
		footer,
	)
}

func (m Model) viewFooter() string {
	hints := map[screen]string{
		screenMenu:       "Up/Down select  Enter confirm  q quit",
		screenEditList:   "Up/Down select  Enter edit  a add  d delete  Esc back",
		screenAddType:    "Up/Down select  Enter confirm  Esc back",
		screenEditForm:   "Tab next field  Space toggle  Left/Right change  Ctrl+S save  Esc cancel",
		screenDNS:        "Tab next field  Ctrl+S save  Esc cancel",
		screenActivate:   "Up/Down select  Enter activate  Esc back",
		screenDeactivate: "Up/Down select  Enter deactivate  Esc back",
		screenConfirm:    "y confirm  n/Esc cancel",
		screenMessage:    "Enter/Esc back",
	}
	h := hints[m.screen]
	path := subtleStyle.Render(m.cfgPath)
	devHint := subtleStyle.Render(fmt.Sprintf("%d system interfaces", len(m.devices)))
	idleHint := ""
	if m.idleTimeout > 0 {
		left := int(m.idleRemaining().Seconds() + 0.999)
		if left < 0 {
			left = 0
		}
		if left <= 10 {
			idleHint = "  " + errorStyle.Render(fmt.Sprintf("idle exit in %ds", left))
		} else {
			idleHint = "  " + subtleStyle.Render(fmt.Sprintf("idle %ds", left))
		}
	}
	return footerStyle.Render(h) + "\n" + path + "  " + devHint + idleHint
}

func (m Model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.menuIdx > 0 {
			m.menuIdx--
		}
	case "down", "j":
		if m.menuIdx < len(menuItems)-1 {
			m.menuIdx++
		}
	case "enter", " ":
		switch m.menuIdx {
		case 0:
			m.reload()
			m.listIdx = 0
			m.screen = screenEditList
		case 1:
			m.startDNSForm()
		case 2:
			m.beginApplyDNSFromFile()
		case 3:
			m.reload()
			m.actIdx = 0
			m.screen = screenActivate
		case 4:
			m.reload()
			m.actIdx = 0
			m.screen = screenDeactivate
		case 5:
			m.confirm = confirmRestartNetworking
			m.screen = screenConfirm
		case 6:
			m.confirm = confirmClearAll
			m.screen = screenConfirm
		case 7:
			dir, err := packages.SelfDir()
			if err != nil {
				m.showMsg("Install failed", err.Error(), screenMenu)
				return m, nil
			}
			found, err := packages.FindBondVLANDebs(dir)
			if err != nil {
				m.showMsg("Install failed", err.Error(), screenMenu)
				return m, nil
			}
			if len(found.Ifenslave) == 0 || len(found.VLAN) == 0 {
				detail := fmt.Sprintf("Searched: %s\n", dir)
				if len(found.Ifenslave) == 0 {
					detail += "Missing: ifenslave_*.deb\n"
				}
				if len(found.VLAN) == 0 {
					detail += "Missing: vlan_*.deb\n"
				}
				if n := len(found.Found()); n > 0 {
					detail += "Partial matches found, but both packages are required."
				} else {
					detail += "Place ifenslave and vlan .deb files next to this binary."
				}
				m.showMsg("Packages not found", detail, screenMenu)
				return m, nil
			}
			m.pendingDebDir = dir
			m.pendingDebs = found.Found()
			m.confirm = confirmInstallDebs
			m.screen = screenConfirm
		case 8:
			m.confirm = confirmClearAptSources
			m.screen = screenConfirm
		case 9:
			dir, err := packages.SelfDir()
			if err != nil {
				m.showMsg("Apply failed", err.Error(), screenMenu)
				return m, nil
			}
			cfgs, err := aptsources.FindLocalConfigs(dir)
			if err != nil {
				m.showMsg("Apply failed", err.Error(), screenMenu)
				return m, nil
			}
			if len(cfgs) == 0 {
				m.showMsg("No apt source files",
					fmt.Sprintf("Searched: %s\n\nPlace one of:\n  sources.list\n  apt-sources.list\n  *.list / *.sources\n  sources.list.d/*\nnext to this binary.\nSee examples/sources.list", dir),
					screenMenu)
				return m, nil
			}
			m.pendingAptDir = dir
			m.pendingAptCfgs = cfgs
			m.confirm = confirmApplyAptSources
			m.screen = screenConfirm
		case 10:
			dir, err := packages.SelfDir()
			if err != nil {
				m.showMsg("SSH setup failed", err.Error(), screenMenu)
				return m, nil
			}
			rc, err := sshsetup.LoadRootConf(dir)
			if err != nil {
				m.showMsg("SSH setup failed", err.Error()+"\n\nSee examples/ssh-root.conf and examples/root.pub", screenMenu)
				return m, nil
			}
			debs, _ := sshsetup.FindSSHDebs(dir)
			m.pendingSSHDir = dir
			m.pendingSSHDebs = debs
			m.pendingSSHPub = rc.PubkeyFile
			if len(debs) > 0 {
				m.pendingSSHMethod = "local .deb"
			} else {
				m.pendingSSHMethod = "apt-get install openssh-server"
			}
			m.confirm = confirmSetupSSH
			m.screen = screenConfirm
		case 11:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) viewMenu() string {
	var b strings.Builder
	b.WriteString(sectionStyle.Render("Main Menu") + "\n\n")
	for i, item := range menuItems {
		cursor := "  "
		style := itemStyle
		if i == m.menuIdx {
			cursor = "> "
			style = selectedStyle
		}
		b.WriteString(style.Render(cursor+item) + "\n")
	}
	return b.String()
}

func (m Model) updateEditList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.screen = screenMenu
		m.status = ""
	case "up", "k":
		if m.listIdx > 0 {
			m.listIdx--
		}
	case "down", "j":
		if m.listIdx < len(m.conns)-1 {
			m.listIdx++
		}
	case "a":
		m.addTypeIdx = 0
		m.screen = screenAddType
	case "d":
		if len(m.conns) == 0 {
			return m, nil
		}
		c := m.conns[m.listIdx]
		if c.Name == "lo" {
			m.status = "Cannot delete lo interface"
			return m, nil
		}
		if !c.Configured() {
			m.status = "Interface is not configured yet"
			return m, nil
		}
		m.confirm = confirmDelete
		m.confirmN = c.Name
		m.screen = screenConfirm
	case "enter", " ":
		if len(m.conns) == 0 {
			m.addTypeIdx = 0
			m.screen = screenAddType
			return m, nil
		}
		c := m.conns[m.listIdx]
		if !c.Configured() {
			// Start ethernet form prefilled with device name
			nc := interfaces.NewConnection(c.Name)
			nc.AllowHotplug = true
			m.startEditForm(nc, true, interfaces.TypeEthernet)
			return m, nil
		}
		m.startEditForm(cloneConn(c), false, c.Type())
	}
	return m, nil
}

func (m Model) viewEditList() string {
	var b strings.Builder
	b.WriteString(sectionStyle.Render("Edit a connection") + "\n\n")
	if len(m.conns) == 0 {
		b.WriteString(subtleStyle.Render("  (No interfaces — press a to add)") + "\n")
		return b.String()
	}
	for i, c := range m.conns {
		cursor := "  "
		style := itemStyle
		if i == m.listIdx {
			cursor = "> "
			style = selectedStyle
		}
		ctype := string(c.Type())
		if !c.Configured() {
			ctype = "unconfigured"
		}
		state := "-"
		addrs := ""
		for _, d := range m.devices {
			if d.Name == c.Name {
				state = strings.ToUpper(d.State)
				if state == "" {
					state = "?"
				}
				addrs = strings.Join(d.Addrs, " ")
				if d.Kind != "" && !c.Configured() {
					ctype = d.Kind
				}
				break
			}
		}
		v4 := string(c.IPv4Method())
		if v4 == "" {
			v4 = "-"
		}
		line := fmt.Sprintf("%s%-14s %-12s %-6s inet:%-7s %s",
			cursor, c.Name, ctype, state, v4, addrs)
		b.WriteString(style.Render(line) + "\n")
	}
	return b.String()
}

func (m Model) updateAddType(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.screen = screenEditList
	case "up", "k":
		if m.addTypeIdx > 0 {
			m.addTypeIdx--
		}
	case "down", "j":
		if m.addTypeIdx < len(addTypeItems)-1 {
			m.addTypeIdx++
		}
	case "enter", " ":
		switch m.addTypeIdx {
		case 0:
			m.startEditForm(interfaces.NewConnection(""), true, interfaces.TypeEthernet)
		case 1:
			c := interfaces.NewConnection("")
			m.startEditForm(c, true, interfaces.TypeVLAN)
		case 2:
			c := &interfaces.Connection{
				Name: "bond0",
				Auto: true,
				IPv4: &interfaces.Iface{
					Name:   "bond0",
					Family: interfaces.FamilyInet,
					Method: interfaces.MethodManual,
					Options: []interfaces.Option{
						{Key: "bond-mode", Value: "802.3ad"},
						{Key: "bond-miimon", Value: "100"},
						{Key: "bond-slaves", Value: ""},
					},
				},
			}
			m.startEditForm(c, true, interfaces.TypeBond)
		}
	}
	return m, nil
}

func (m Model) viewAddType() string {
	var b strings.Builder
	b.WriteString(sectionStyle.Render("Add connection — choose type") + "\n\n")
	for i, item := range addTypeItems {
		cursor := "  "
		style := itemStyle
		if i == m.addTypeIdx {
			cursor = "> "
			style = selectedStyle
		}
		b.WriteString(style.Render(cursor+item) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("  VLAN parent can be ethX or bondX (VLAN on bond).") + "\n")
	return b.String()
}

func cloneConn(c *interfaces.Connection) *interfaces.Connection {
	if c == nil {
		return nil
	}
	cp := *c
	if c.IPv4 != nil {
		v := *c.IPv4
		v.Options = append([]interfaces.Option{}, c.IPv4.Options...)
		cp.IPv4 = &v
	}
	if c.IPv6 != nil {
		v := *c.IPv6
		v.Options = append([]interfaces.Option{}, c.IPv6.Options...)
		cp.IPv6 = &v
	}
	return &cp
}

func (m Model) deviceNamesHint() string {
	var names []string
	for _, d := range m.devices {
		if d.IsLoop {
			continue
		}
		names = append(names, d.Name)
	}
	if len(names) == 0 {
		return "(no system interfaces detected)"
	}
	if len(names) > 8 {
		return strings.Join(names[:8], " ") + " ..."
	}
	return strings.Join(names, " ")
}
