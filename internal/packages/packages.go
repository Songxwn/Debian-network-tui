package packages

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// DebFindResult holds discovered local .deb packages beside the binary.
type DebFindResult struct {
	Dir       string
	Ifenslave []string // absolute paths
	VLAN      []string
	NetTools  []string
}

// Found returns all discovered package paths (ifenslave, vlan, net-tools).
func (r DebFindResult) Found() []string {
	out := append([]string{}, r.Ifenslave...)
	out = append(out, r.VLAN...)
	out = append(out, r.NetTools...)
	return out
}

// SelfDir returns the directory containing the running executable.
func SelfDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve executable symlink: %w", err)
	}
	return filepath.Dir(exe), nil
}

// FindBondVLANDebs searches dir for local ifenslave / vlan / net-tools .deb packages.
func FindBondVLANDebs(dir string) (DebFindResult, error) {
	res := DebFindResult{Dir: dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res, fmt.Errorf("read directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".deb") {
			continue
		}
		abs := filepath.Join(dir, name)
		switch {
		case strings.Contains(lower, "ifenslave"):
			res.Ifenslave = append(res.Ifenslave, abs)
		case strings.Contains(lower, "net-tools") || strings.Contains(lower, "net_tools"):
			res.NetTools = append(res.NetTools, abs)
		case strings.Contains(lower, "vlan"):
			res.VLAN = append(res.VLAN, abs)
		}
	}
	sort.Strings(res.Ifenslave)
	sort.Strings(res.VLAN)
	sort.Strings(res.NetTools)
	return res, nil
}

// InstallLocalDebs installs the given .deb files with apt-get.
func InstallLocalDebs(debs []string) (string, error) {
	if len(debs) == 0 {
		return "", fmt.Errorf("no .deb packages to install")
	}
	apt, err := exec.LookPath("apt-get")
	if err != nil {
		apt, err = exec.LookPath("apt")
		if err != nil {
			return "", fmt.Errorf("apt-get/apt not found")
		}
	}

	args := []string{"install", "-y", "--allow-downgrades"}
	args = append(args, debs...)

	cmd := exec.Command(apt, args...)
	cmd.Env = append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"NEEDRESTART_MODE=a",
	)
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			return "", fmt.Errorf("apt install failed: %w", err)
		}
		return msg, fmt.Errorf("apt install failed: %s", msg)
	}
	if msg == "" {
		msg = "Packages installed successfully."
	}
	return msg, nil
}

// MissingRequired returns human-readable patterns for required packages that were not found.
func (r DebFindResult) MissingRequired() []string {
	var miss []string
	if len(r.Ifenslave) == 0 {
		miss = append(miss, "ifenslave_*.deb")
	}
	if len(r.VLAN) == 0 {
		miss = append(miss, "vlan_*.deb")
	}
	if len(r.NetTools) == 0 {
		miss = append(miss, "net-tools_*.deb")
	}
	return miss
}

// InstallBondVLANFromSelfDir finds and installs ifenslave/vlan/net-tools debs next to the binary.
func InstallBondVLANFromSelfDir() (DebFindResult, string, error) {
	dir, err := SelfDir()
	if err != nil {
		return DebFindResult{}, "", err
	}
	found, err := FindBondVLANDebs(dir)
	if err != nil {
		return found, "", err
	}
	if miss := found.MissingRequired(); len(miss) > 0 {
		return found, "", fmt.Errorf("missing packages in %s: %s", dir, strings.Join(miss, ", "))
	}
	msg, err := InstallLocalDebs(found.Found())
	return found, msg, err
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}
