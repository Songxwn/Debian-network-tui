package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/debian-network-tui/debian-network-tui/internal/interfaces"
	"github.com/debian-network-tui/debian-network-tui/internal/netdev"
)

type screen int

const (
	screenMenu screen = iota
	screenEditList
	screenEditForm
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
	formFocus int
	actIdx    int

	conns    []*interfaces.Connection
	devices  []netdev.Device
	editConn *interfaces.Connection
	editNew  bool
	inputs   []textinput.Model

	// Form toggles (not free-text)
	autoOn     bool
	hotplugOn  bool
	ipv4Method int // 0=dhcp 1=static 2=disabled
	ipv6Method int // 0=disabled 1=dhcp 2=static 3=auto(manual accept_ra style -> dhcp for simplicity) 

	status   string
	errMsg   string
	confirm  confirmAction
	confirmN string // target name
	msgTitle string
	msgBody  string
	msgBack  screen

	quit bool
}

var menuItems = []string{
	"编辑连接 (Edit a connection)",
	"激活连接 (Activate a connection)",
	"停用连接 (Deactivate a connection)",
	"退出 (Quit)",
}

func New(cfgPath string) Model {
	m := Model{
		cfgPath: cfgPath,
		screen:  screenMenu,
	}
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
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.quit {
			return m, tea.Quit
		}
		switch m.screen {
		case screenMenu:
			return m.updateMenu(msg)
		case screenEditList:
			return m.updateEditList(msg)
		case screenEditForm:
			return m.updateEditForm(msg)
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
	case screenEditForm:
		body = m.viewEditForm()
	case screenActivate:
		body = m.viewActList(true)
	case screenDeactivate:
		body = m.viewActList(false)
	case screenConfirm:
		body = m.viewConfirm()
	case screenMessage:
		body = m.viewMessage()
	default:
		body = "未知界面"
	}

	header := titleStyle.Render("debian-network-tui") + "  " +
		subtleStyle.Render("管理 /etc/network/interfaces · Debian 11–13")
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
		screenMenu:       "↑/↓ 选择  Enter 确认  q 退出",
		screenEditList:   "↑/↓ 选择  Enter 编辑  a 新建  d 删除  Esc 返回",
		screenEditForm:   "Tab 切换字段  ←/→ 改选项  Ctrl+S 保存  Esc 取消",
		screenActivate:   "↑/↓ 选择  Enter 激活  Esc 返回",
		screenDeactivate: "↑/↓ 选择  Enter 停用  Esc 返回",
		screenConfirm:    "y 确认  n/Esc 取消",
		screenMessage:    "Enter/Esc 返回",
	}
	h := hints[m.screen]
	path := subtleStyle.Render(m.cfgPath)
	return footerStyle.Render(h) + "\n" + path
}

// ---------- Menu ----------

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
			m.reload()
			m.actIdx = 0
			m.screen = screenActivate
		case 2:
			m.reload()
			m.actIdx = 0
			m.screen = screenDeactivate
		case 3:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) viewMenu() string {
	var b strings.Builder
	b.WriteString(sectionStyle.Render("主菜单") + "\n\n")
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

// ---------- Edit list ----------

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
		m.startNewForm()
	case "d":
		if len(m.conns) == 0 {
			return m, nil
		}
		name := m.conns[m.listIdx].Name
		if name == "lo" {
			m.status = "不能删除 lo 接口"
			return m, nil
		}
		m.confirm = confirmDelete
		m.confirmN = name
		m.screen = screenConfirm
	case "enter", " ":
		if len(m.conns) == 0 {
			m.startNewForm()
			return m, nil
		}
		m.startEditForm(cloneConn(m.conns[m.listIdx]), false)
	}
	return m, nil
}

func (m Model) viewEditList() string {
	var b strings.Builder
	b.WriteString(sectionStyle.Render("编辑连接") + "\n\n")
	if len(m.conns) == 0 {
		b.WriteString(subtleStyle.Render("  (无连接，按 a 新建)") + "\n")
		return b.String()
	}
	for i, c := range m.conns {
		cursor := "  "
		style := itemStyle
		if i == m.listIdx {
			cursor = "> "
			style = selectedStyle
		}
		flags := []string{}
		if c.Auto {
			flags = append(flags, "auto")
		}
		if c.AllowHotplug {
			flags = append(flags, "hotplug")
		}
		v4 := string(c.IPv4Method())
		if v4 == "" {
			v4 = "-"
		}
		v6 := string(c.IPv6Method())
		if v6 == "" {
			v6 = "-"
		}
		line := fmt.Sprintf("%s%-12s  inet:%-8s inet6:%-8s  %s",
			cursor, c.Name, v4, v6, strings.Join(flags, ","))
		b.WriteString(style.Render(line) + "\n")
	}
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
