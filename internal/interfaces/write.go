package interfaces

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SaveConnection writes/updates a connection into the primary interfaces file.
// It rebuilds auto/allow-hotplug/iface stanzas for that name while preserving
// unrelated content. Creates a timestamped backup before writing.
// For bond interfaces, slave stanzas (inet manual + bond-master) are also written.
func (f *File) SaveConnection(conn *Connection) error {
	if conn == nil || conn.Name == "" {
		return fmt.Errorf("invalid connection")
	}
	f.applyConnection(conn)
	if conn.Type() == TypeBond {
		for _, slave := range conn.BondSlaves() {
			slaveConn := &Connection{
				Name: slave,
				Auto: true,
				IPv4: &Iface{
					Name:   slave,
					Family: FamilyInet,
					Method: MethodManual,
					Options: []Option{
						{Key: "bond-master", Value: conn.Name},
					},
				},
			}
			f.applyConnection(slaveConn)
		}
	}
	return f.writeAtomic(f.Path)
}

// DeleteConnection removes auto/allow-hotplug/iface stanzas for name.
func (f *File) DeleteConnection(name string) error {
	if name == "" || name == "lo" {
		return fmt.Errorf("cannot delete interface %q", name)
	}
	var kept []Stanza
	for _, s := range f.Stanzas {
		switch s.Kind {
		case KindAuto, KindAllowHotplug:
			var names []string
			for _, n := range s.Names {
				if n != name {
					names = append(names, n)
				}
			}
			if len(names) == 0 {
				continue
			}
			s.Names = names
			s.Raw = rebuildAutoRaw(s)
			kept = append(kept, s)
		case KindIface:
			if s.Iface != nil && s.Iface.Name == name {
				continue
			}
			kept = append(kept, s)
		default:
			kept = append(kept, s)
		}
	}
	f.Stanzas = kept
	return f.writeAtomic(f.Path)
}

func rebuildAutoRaw(s Stanza) string {
	prefix := "auto"
	if s.Kind == KindAllowHotplug {
		prefix = "allow-hotplug"
	}
	return prefix + " " + strings.Join(s.Names, " ")
}

func (f *File) applyConnection(conn *Connection) {
	name := conn.Name
	var kept []Stanza
	for _, s := range f.Stanzas {
		switch s.Kind {
		case KindAuto, KindAllowHotplug:
			var names []string
			for _, n := range s.Names {
				if n != name {
					names = append(names, n)
				}
			}
			if len(names) == 0 {
				continue
			}
			s.Names = names
			s.Raw = rebuildAutoRaw(s)
			kept = append(kept, s)
		case KindIface:
			if s.Iface != nil && s.Iface.Name == name {
				continue
			}
			kept = append(kept, s)
		default:
			kept = append(kept, s)
		}
	}

	// Insert new stanzas before trailing blanks, after existing content.
	insert := buildConnectionStanzas(conn)
	f.Stanzas = append(kept, insert...)
}

func buildConnectionStanzas(conn *Connection) []Stanza {
	var out []Stanza
	out = append(out, Stanza{Kind: KindBlank, Raw: ""})
	out = append(out, Stanza{
		Kind: KindComment,
		Raw:  fmt.Sprintf("# Managed by debian-network-tui: %s", conn.Name),
	})
	if conn.Auto {
		out = append(out, Stanza{
			Kind:  KindAuto,
			Names: []string{conn.Name},
			Raw:   "auto " + conn.Name,
		})
	}
	if conn.AllowHotplug {
		out = append(out, Stanza{
			Kind:  KindAllowHotplug,
			Names: []string{conn.Name},
			Raw:   "allow-hotplug " + conn.Name,
		})
	}
	if conn.IPv4 != nil {
		out = append(out, ifaceStanza(conn.IPv4))
	}
	if conn.IPv6 != nil {
		out = append(out, ifaceStanza(conn.IPv6))
	}
	for _, e := range conn.Extra {
		out = append(out, ifaceStanza(e))
	}
	return out
}

func ifaceStanza(i *Iface) Stanza {
	s := Stanza{
		Kind:  KindIface,
		Iface: i,
		Raw:   fmt.Sprintf("iface %s %s %s", i.Name, i.Family, i.Method),
	}
	return s
}

// Render serializes the file to interfaces(5) text.
func (f *File) Render() string {
	var b strings.Builder
	for i, s := range f.Stanzas {
		switch s.Kind {
		case KindBlank:
			b.WriteString("\n")
		case KindComment, KindOther, KindMapping, KindSource, KindSourceDirectory, KindAllow:
			b.WriteString(s.Raw)
			b.WriteString("\n")
		case KindAuto:
			b.WriteString("auto " + strings.Join(s.Names, " ") + "\n")
		case KindAllowHotplug:
			b.WriteString("allow-hotplug " + strings.Join(s.Names, " ") + "\n")
		case KindIface:
			if s.Iface == nil {
				b.WriteString(s.Raw + "\n")
				continue
			}
			b.WriteString(fmt.Sprintf("iface %s %s %s\n", s.Iface.Name, s.Iface.Family, s.Iface.Method))
			for _, o := range s.Iface.Options {
				if o.Key == "" {
					continue
				}
				if o.Value == "" {
					b.WriteString(fmt.Sprintf("    %s\n", o.Key))
				} else {
					b.WriteString(fmt.Sprintf("    %s %s\n", o.Key, o.Value))
				}
			}
		}
		_ = i
	}
	return b.String()
}

func (f *File) writeAtomic(path string) error {
	content := f.Render()
	if err := backupFile(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".interfaces-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		// Non-fatal on some FS
		_ = err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config file: %w", err)
	}
	return nil
}

func backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read for backup: %w", err)
	}
	bak := fmt.Sprintf("%s.bak.%s", path, time.Now().Format("20060102-150405"))
	if err := os.WriteFile(bak, data, 0o644); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

// NewConnection builds a default DHCP connection.
func NewConnection(name string) *Connection {
	return &Connection{
		Name:         name,
		AllowHotplug: true,
		IPv4: &Iface{
			Name:   name,
			Family: FamilyInet,
			Method: MethodDHCP,
		},
	}
}

// EnsureIPv4 creates an inet iface if missing.
func (c *Connection) EnsureIPv4(method Method) {
	if c.IPv4 == nil {
		c.IPv4 = &Iface{Name: c.Name, Family: FamilyInet, Method: method}
		return
	}
	c.IPv4.Name = c.Name
	c.IPv4.Family = FamilyInet
	c.IPv4.Method = method
}

// EnsureIPv6 creates an inet6 iface if missing.
func (c *Connection) EnsureIPv6(method Method) {
	if c.IPv6 == nil {
		c.IPv6 = &Iface{Name: c.Name, Family: FamilyInet6, Method: method}
		return
	}
	c.IPv6.Name = c.Name
	c.IPv6.Family = FamilyInet6
	c.IPv6.Method = method
}

// ClearIPv4 removes IPv4 stanza.
func (c *Connection) ClearIPv4() { c.IPv4 = nil }

// ClearIPv6 removes IPv6 stanza.
func (c *Connection) ClearIPv6() { c.IPv6 = nil }
