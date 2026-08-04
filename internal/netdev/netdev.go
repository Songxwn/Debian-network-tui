package netdev

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Device is a network interface present on the system.
type Device struct {
	Name   string
	State  string // UP / DOWN / UNKNOWN
	MAC    string
	Addrs  []string
	IsLoop bool
}

// ListDevices returns non-virtual-ish NICs from /sys/class/net.
func ListDevices() ([]Device, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, fmt.Errorf("list network devices: %w", err)
	}
	var out []Device
	for _, e := range entries {
		name := e.Name()
		d := Device{
			Name:   name,
			IsLoop: name == "lo",
			State:  readFile(filepath.Join("/sys/class/net", name, "operstate")),
			MAC:    readFile(filepath.Join("/sys/class/net", name, "address")),
		}
		d.Addrs = readAddresses(name)
		out = append(out, d)
	}
	return out, nil
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
	// Format: eth0 UP 192.168.1.10/24 fe80::1/64
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
	// Unknown / dormant: treat as up if IPv4/IPv6 address present.
	return len(readAddresses(name)) > 0
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

// ReloadNetworking restarts networking via systemctl when available.
func ReloadNetworking() error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command("systemctl", "restart", "networking")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl restart networking failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	return fmt.Errorf("systemctl not found; run ifdown/ifup manually")
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
