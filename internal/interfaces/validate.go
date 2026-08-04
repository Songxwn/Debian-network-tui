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
		return fmt.Errorf("连接为空")
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return fmt.Errorf("接口名不能为空")
	}
	if strings.ContainsAny(name, " \t/") {
		return fmt.Errorf("接口名含非法字符")
	}
	if c.IPv4 == nil && c.IPv6 == nil {
		return fmt.Errorf("至少配置 IPv4 或 IPv6")
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
			return fmt.Errorf("%s: static 模式需要 address", i.Name)
		}
		if v6 {
			host := addr
			if strings.Contains(addr, "/") {
				var pref string
				var ok bool
				host, pref, ok = strings.Cut(addr, "/")
				if !ok {
					return fmt.Errorf("%s: 无效的 IPv6 地址", i.Name)
				}
				if _, err := strconv.Atoi(pref); err != nil {
					return fmt.Errorf("%s: 无效的 IPv6 前缀长度", i.Name)
				}
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.To4() != nil {
				return fmt.Errorf("%s: 无效的 IPv6 地址", i.Name)
			}
			gw := i.GetOption("gateway")
			if gw != "" {
				gip := net.ParseIP(gw)
				if gip == nil || gip.To4() != nil {
					return fmt.Errorf("%s: 无效的 IPv6 网关", i.Name)
				}
			}
		} else {
			if !reIPv4.MatchString(addr) {
				return fmt.Errorf("%s: 无效的 IPv4 地址 %q", i.Name, addr)
			}
			nm := i.GetOption("netmask")
			if nm != "" && !reIPv4.MatchString(nm) {
				return fmt.Errorf("%s: 无效的子网掩码 %q", i.Name, nm)
			}
			gw := i.GetOption("gateway")
			if gw != "" && !reIPv4.MatchString(gw) {
				return fmt.Errorf("%s: 无效的网关 %q", i.Name, gw)
			}
		}
		return nil
	default:
		return fmt.Errorf("%s: 不支持的 method %q", i.Name, i.Method)
	}
}
