package interfaces

import (
	"strconv"
	"strings"
)

// Method is the iface address family method (dhcp, static, ...).
type Method string

const (
	MethodDHCP     Method = "dhcp"
	MethodStatic   Method = "static"
	MethodManual   Method = "manual"
	MethodLoopback Method = "loopback"
)

// Family is inet or inet6.
type Family string

const (
	FamilyInet  Family = "inet"
	FamilyInet6 Family = "inet6"
)

// ConnType is the logical connection type shown in the UI.
type ConnType string

const (
	TypeEthernet ConnType = "ethernet"
	TypeVLAN     ConnType = "vlan"
	TypeBond     ConnType = "bond"
)

// StanzaKind classifies top-level stanzas.
type StanzaKind int

const (
	KindComment StanzaKind = iota
	KindBlank
	KindAuto
	KindAllowHotplug
	KindAllow
	KindIface
	KindMapping
	KindSource
	KindSourceDirectory
	KindOther
)

// Option is an indented key/value under an iface stanza.
type Option struct {
	Key   string
	Value string
}

// Iface describes one "iface NAME FAMILY METHOD" block.
type Iface struct {
	Name    string
	Family  Family
	Method  Method
	Options []Option
}

// Connection aggregates all iface stanzas (inet/inet6) for one device name,
// plus auto / allow-hotplug flags.
type Connection struct {
	Name         string
	Auto         bool
	AllowHotplug bool
	IPv4         *Iface
	IPv6         *Iface
	// Extra holds non-inet/inet6 iface blocks for the same name (rare).
	Extra []*Iface
	// FromSystem is true when this entry was synthesized from a live NIC
	// that has no stanza in the interfaces file yet.
	FromSystem bool
}

// File is a parsed interfaces document.
type File struct {
	Path    string
	Stanzas []Stanza
	// Sources are paths pulled in via source / source-directory (resolved files).
	Sources []*File
}

// Stanza is one logical unit in the file.
type Stanza struct {
	Kind    StanzaKind
	Raw     string   // for comment/blank/other/source lines
	Names   []string // auto / allow-hotplug / allow
	Iface   *Iface
	Path    string // source / source-directory argument
	Allow   string // allow-<CLASS>
}

// GetOption returns the first option value for key (case-sensitive as stored).
func (i *Iface) GetOption(key string) string {
	if i == nil {
		return ""
	}
	for _, o := range i.Options {
		if o.Key == key {
			return o.Value
		}
	}
	return ""
}

// SetOption sets or replaces an option; empty value removes it.
func (i *Iface) SetOption(key, value string) {
	if i == nil {
		return
	}
	for idx, o := range i.Options {
		if o.Key == key {
			if value == "" {
				i.Options = append(i.Options[:idx], i.Options[idx+1:]...)
				return
			}
			i.Options[idx].Value = value
			return
		}
	}
	if value != "" {
		i.Options = append(i.Options, Option{Key: key, Value: value})
	}
}

// HasOption reports whether key exists.
func (i *Iface) HasOption(key string) bool {
	if i == nil {
		return false
	}
	for _, o := range i.Options {
		if o.Key == key {
			return true
		}
	}
	return false
}

// Type detects ethernet / vlan / bond from options and name.
func (c *Connection) Type() ConnType {
	if c == nil {
		return TypeEthernet
	}
	get := func(key string) string {
		if c.IPv4 != nil {
			if v := c.IPv4.GetOption(key); v != "" {
				return v
			}
		}
		if c.IPv6 != nil {
			if v := c.IPv6.GetOption(key); v != "" {
				return v
			}
		}
		return ""
	}
	if get("bond-slaves") != "" || (get("bond-mode") != "" && get("bond-master") == "") {
		return TypeBond
	}
	if get("vlan-raw-device") != "" || get("vlan_id") != "" || get("vlan-id") != "" {
		return TypeVLAN
	}
	if strings.Contains(c.Name, ".") {
		return TypeVLAN
	}
	if strings.HasPrefix(c.Name, "bond") && get("bond-master") == "" {
		return TypeBond
	}
	return TypeEthernet
}

// VLANParent returns vlan-raw-device or parent from dotted name.
func (c *Connection) VLANParent() string {
	if c.IPv4 != nil {
		if p := c.IPv4.GetOption("vlan-raw-device"); p != "" {
			return p
		}
	}
	if c.IPv6 != nil {
		if p := c.IPv6.GetOption("vlan-raw-device"); p != "" {
			return p
		}
	}
	if i := strings.LastIndex(c.Name, "."); i > 0 {
		return c.Name[:i]
	}
	return ""
}

// VLANID returns vlan id from option or dotted name.
func (c *Connection) VLANID() string {
	for _, iface := range []*Iface{c.IPv4, c.IPv6} {
		if iface == nil {
			continue
		}
		if v := iface.GetOption("vlan_id"); v != "" {
			return v
		}
		if v := iface.GetOption("vlan-id"); v != "" {
			return v
		}
	}
	if i := strings.LastIndex(c.Name, "."); i > 0 {
		id := c.Name[i+1:]
		if _, err := strconv.Atoi(id); err == nil {
			return id
		}
	}
	return ""
}

// BondSlaves returns bond-slaves list.
func (c *Connection) BondSlaves() []string {
	var raw string
	if c.IPv4 != nil {
		raw = c.IPv4.GetOption("bond-slaves")
	}
	if raw == "" && c.IPv6 != nil {
		raw = c.IPv6.GetOption("bond-slaves")
	}
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}

// BondMode returns bond-mode.
func (c *Connection) BondMode() string {
	if c.IPv4 != nil {
		if v := c.IPv4.GetOption("bond-mode"); v != "" {
			return v
		}
	}
	if c.IPv6 != nil {
		return c.IPv6.GetOption("bond-mode")
	}
	return ""
}

// IPv4Method helpers.
func (c *Connection) IPv4Method() Method {
	if c.IPv4 == nil {
		return ""
	}
	return c.IPv4.Method
}

func (c *Connection) IPv6Method() Method {
	if c.IPv6 == nil {
		return ""
	}
	return c.IPv6.Method
}

// Configured reports whether the connection has any iface stanza.
func (c *Connection) Configured() bool {
	return c != nil && (c.IPv4 != nil || c.IPv6 != nil || len(c.Extra) > 0)
}
