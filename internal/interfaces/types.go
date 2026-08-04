package interfaces

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

// Address helpers.
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
