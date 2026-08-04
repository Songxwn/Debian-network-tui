package interfaces

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var reIPv4 = regexp.MustCompile(`^((25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(25[0-5]|2[0-4]\d|[01]?\d\d?)$`)

// ValidateConnection checks fields before save.
func ValidateConnection(c *Connection) error {
	if c == nil {
		return fmt.Errorf("connection is nil")
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return fmt.Errorf("interface name is required")
	}
	if strings.ContainsAny(name, " \t/") {
		return fmt.Errorf("interface name contains invalid characters")
	}
	if c.IPv4 == nil && c.IPv6 == nil {
		return fmt.Errorf("configure at least IPv4 or IPv6")
	}

	switch c.Type() {
	case TypeVLAN:
		parent := c.VLANParent()
		if parent == "" {
			return fmt.Errorf("VLAN requires parent device (vlan-raw-device)")
		}
		id := c.VLANID()
		if id == "" {
			return fmt.Errorf("VLAN requires a VLAN ID")
		}
		n, err := strconv.Atoi(id)
		if err != nil || n < 2 || n > 4094 {
			return fmt.Errorf("VLAN ID must be 2-4094")
		}
	case TypeBond:
		slaves := c.BondSlaves()
		if len(slaves) == 0 {
			return fmt.Errorf("bond requires at least one slave (bond-slaves)")
		}
		for _, s := range slaves {
			if s == name {
				return fmt.Errorf("bond cannot enslave itself")
			}
		}
		if c.BondMode() == "" {
			return fmt.Errorf("bond requires bond-mode")
		}
	}

	if c.IPv4 != nil {
		if err := validateIface(c.IPv4, false); err != nil {
			return err
		}
	}
	if c.IPv6 != nil {
		if err := validateIface(c.IPv6, true); err != nil {
			return err
		}
	}
	return nil
}

func validateIface(i *Iface, v6 bool) error {
	switch i.Method {
	case MethodDHCP, MethodManual, MethodLoopback:
		return nil
	case MethodStatic:
		addr := i.GetOption("address")
		if addr == "" {
			return fmt.Errorf("%s: static method requires address", i.Name)
		}
		if v6 {
			host := addr
			if strings.Contains(addr, "/") {
				var pref string
				var ok bool
				host, pref, ok = strings.Cut(addr, "/")
				if !ok {
					return fmt.Errorf("%s: invalid IPv6 address", i.Name)
				}
				if _, err := strconv.Atoi(pref); err != nil {
					return fmt.Errorf("%s: invalid IPv6 prefix length", i.Name)
				}
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.To4() != nil {
				return fmt.Errorf("%s: invalid IPv6 address", i.Name)
			}
			gw := i.GetOption("gateway")
			if gw != "" {
				gip := net.ParseIP(gw)
				if gip == nil || gip.To4() != nil {
					return fmt.Errorf("%s: invalid IPv6 gateway", i.Name)
				}
			}
		} else {
			if !reIPv4.MatchString(addr) {
				return fmt.Errorf("%s: invalid IPv4 address %q", i.Name, addr)
			}
			nm := i.GetOption("netmask")
			if nm != "" && !reIPv4.MatchString(nm) {
				return fmt.Errorf("%s: invalid netmask %q", i.Name, nm)
			}
			gw := i.GetOption("gateway")
			if gw != "" && !reIPv4.MatchString(gw) {
				return fmt.Errorf("%s: invalid gateway %q", i.Name, gw)
			}
		}
		return nil
	default:
		return fmt.Errorf("%s: unsupported method %q", i.Name, i.Method)
	}
}
