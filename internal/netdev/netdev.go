package netdev

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Device is a network interface present on the system.
type Device struct {
	Name   string
	State  string // UP / DOWN / UNKNOWN
	MAC    string
	Addrs  []string
	IsLoop bool
	Kind   string // ethernet, vlan, bond, bridge, other
}

// ListDevices returns NICs from /sys/class/net (all except empty names).
func ListDevices() ([]Device, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		// Fallback for non-Linux build/test hosts.
		names, perr := ParseProcNetDev()
		if perr != nil {
			return nil, fmt.Errorf("list network devices: %w", err)
		}
		out := make([]Device, 0, len(names))
		for _, name := range names {
			out = append(out, Device{Name: name, IsLoop: name == "lo", Kind: "ethernet"})
		}
		return out, nil
	}
	var out []Device
	for _, e := range entries {
		name := e.Name()
		if name == "" {
			continue
		}
		d := Device{
			Name:   name,
			IsLoop: name == "lo",
			State:  readFile(filepath.Join("/sys/class/net", name, "operstate")),
			MAC:    readFile(filepath.Join("/sys/class/net", name, "address")),
			Kind:   detectKind(name),
		}
		d.Addrs = readAddresses(name)
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ListPhysicalCandidates returns non-loop devices usable as ethernet slaves / VLAN parents.
func ListPhysicalCandidates() ([]Device, error) {
	all, err := ListDevices()
	if err != nil {
		return nil, err
	}
	var out []Device
	for _, d := range all {
		if d.IsLoop {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func detectKind(name string) string {
	base := filepath.Join("/sys/class/net", name)
	if _, err := os.Stat(filepath.Join(base, "bonding")); err == nil {
		return "bond"
	}
	if _, err := os.Stat(filepath.Join(base, "bridge")); err == nil {
		return "bridge"
	}
	uevent := readFile(filepath.Join(base, "uevent"))
	for _, line := range strings.Split(uevent, "\n") {
		if strings.HasPrefix(line, "DEVTYPE=") {
			t := strings.TrimPrefix(line, "DEVTYPE=")
			switch t {
			case "vlan", "bond", "bridge", "wlan":
				return t
			}
		}
	}
	if strings.Contains(name, ".") {
		return "vlan"
	}
	if strings.HasPrefix(name, "bond") {
		return "bond"
	}
	if strings.HasPrefix(name, "br") {
		return "bridge"
	}
	return "ethernet"
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readAddresses(name string) []string {
	cmd := exec.Command("ip", "-br", "addr", "show", "dev", name)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(out))
	if len(fields) <= 2 {
		return nil
	}
	return fields[2:]
}

// IsUp reports whether the interface is operatively up.
func IsUp(name string) bool {
	state := strings.ToLower(readFile(filepath.Join("/sys/class/net", name, "operstate")))
	switch state {
	case "up":
		return true
	case "down", "lowerlayerdown", "notpresent":
		return false
	}
	return len(readAddresses(name)) > 0
}

// Exists reports whether the interface exists in sysfs.
func Exists(name string) bool {
	_, err := os.Stat(filepath.Join("/sys/class/net", name))
	return err == nil
}

// IfUp brings an interface up via ifup(8).
func IfUp(name string) error {
	return runIfCmd("ifup", name)
}

// IfDown brings an interface down via ifdown(8).
func IfDown(name string) error {
	return runIfCmd("ifdown", name)
}

func runIfCmd(bin, name string) error {
	cmd := exec.Command(bin, name)
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			return fmt.Errorf("%s %s failed: %w", bin, name, err)
		}
		return fmt.Errorf("%s %s failed: %s", bin, name, msg)
	}
	return nil
}

// ReloadNetworking restarts the ifupdown networking service.
// Tries systemctl first, then /etc/init.d/networking.
func ReloadNetworking() error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command("systemctl", "restart", "networking")
		out, err := cmd.CombinedOutput()
		msg := strings.TrimSpace(string(out))
		if err == nil {
			return nil
		}
		// Fall through to SysV init script on failure.
		if initErr := restartNetworkingInit(); initErr == nil {
			return nil
		}
		if msg == "" {
			return fmt.Errorf("systemctl restart networking failed: %w", err)
		}
		return fmt.Errorf("systemctl restart networking failed: %s", msg)
	}
	return restartNetworkingInit()
}

func restartNetworkingInit() error {
	script := "/etc/init.d/networking"
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("networking service not found (tried systemctl and %s)", script)
	}
	cmd := exec.Command(script, "restart")
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			return fmt.Errorf("%s restart failed: %w", script, err)
		}
		return fmt.Errorf("%s restart failed: %s", script, msg)
	}
	return nil
}

// ParseProcNetDev lists interface names from /proc/net/dev (fallback).
func ParseProcNetDev() ([]string, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var names []string
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if lineNo <= 2 {
			continue
		}
		line := strings.TrimSpace(sc.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 1 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name != "" {
			names = append(names, name)
		}
	}
	return names, sc.Err()
}
