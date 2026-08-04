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
	Dir      string
	Ifenslave []string // absolute paths
	VLAN     []string
}

// Found returns all discovered package paths.
func (r DebFindResult) Found() []string {
	out := append([]string{}, r.Ifenslave...)
	out = append(out, r.VLAN...)
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

// FindBondVLANDebs searches dir for local ifenslave / vlan .deb packages.
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
		case strings.Contains(lower, "vlan"):
			res.VLAN = append(res.VLAN, abs)
		}
	}
	sort.Strings(res.Ifenslave)
	sort.Strings(res.VLAN)
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

// InstallBondVLANFromSelfDir finds and installs ifenslave/vlan debs next to the binary.
func InstallBondVLANFromSelfDir() (DebFindResult, string, error) {
	dir, err := SelfDir()
	if err != nil {
		return DebFindResult{}, "", err
	}
	found, err := FindBondVLANDebs(dir)
	if err != nil {
		return found, "", err
	}
	debs := found.Found()
	if len(debs) == 0 {
		return found, "", fmt.Errorf("no ifenslave/vlan .deb found in %s", dir)
	}
	if len(found.Ifenslave) == 0 {
		return found, "", fmt.Errorf("ifenslave .deb not found in %s (found: %v)", dir, basenames(debs))
	}
	if len(found.VLAN) == 0 {
		return found, "", fmt.Errorf("vlan .deb not found in %s (found: %v)", dir, basenames(debs))
	}
	msg, err := InstallLocalDebs(debs)
	return found, msg, err
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}
