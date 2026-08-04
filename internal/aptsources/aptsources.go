package aptsources

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultSourcesList  = "/etc/apt/sources.list"
	defaultSourcesListD = "/etc/apt/sources.list.d"
)

// Manager operates on apt source paths (overridable for tests).
type Manager struct {
	SourcesList  string
	SourcesListD string
}

// Default returns a manager for the system apt paths.
func Default() Manager {
	return Manager{
		SourcesList:  defaultSourcesList,
		SourcesListD: defaultSourcesListD,
	}
}

// LocalConfig describes a sources file found next to the binary.
type LocalConfig struct {
	Path      string
	TargetRel string // destination under SourcesList or SourcesListD
	IsPrimary bool   // true → write to SourcesList
}

// FindLocalConfigs looks for apt source files beside the binary.
// Recognized names:
//   - sources.list / apt-sources.list  → /etc/apt/sources.list
//   - *.list / *.sources               → /etc/apt/sources.list.d/<name>
//   - sources.list.d/*                 → /etc/apt/sources.list.d/<name>
func FindLocalConfigs(dir string) ([]LocalConfig, error) {
	var out []LocalConfig
	seen := map[string]bool{}

	add := func(path string, primary bool) {
		base := filepath.Base(path)
		key := base
		if primary {
			key = "__primary__"
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, LocalConfig{
			Path:      path,
			TargetRel: base,
			IsPrimary: primary,
		})
	}

	candidates := []string{
		filepath.Join(dir, "sources.list"),
		filepath.Join(dir, "apt-sources.list"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			add(p, true)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if name == "sources.list" || name == "apt-sources.list" {
			continue // already handled as primary
		}
		if strings.HasSuffix(lower, ".list") || strings.HasSuffix(lower, ".sources") {
			add(filepath.Join(dir, name), false)
		}
	}

	sub := filepath.Join(dir, "sources.list.d")
	if subEntries, err := os.ReadDir(sub); err == nil {
		for _, e := range subEntries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			lower := strings.ToLower(name)
			if strings.HasSuffix(lower, ".list") || strings.HasSuffix(lower, ".sources") {
				add(filepath.Join(sub, name), false)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].IsPrimary != out[j].IsPrimary {
			return out[i].IsPrimary
		}
		return out[i].TargetRel < out[j].TargetRel
	})
	return out, nil
}

// Clear removes active apt source configuration after backing up.
// sources.list is replaced with a short comment file; drop-ins in sources.list.d
// ending in .list / .sources are backed up and removed.
func (m Manager) Clear() (string, error) {
	var notes []string
	ts := time.Now().Format("20060102-150405")

	if err := os.MkdirAll(m.SourcesListD, 0o755); err != nil {
		return "", fmt.Errorf("ensure %s: %w", m.SourcesListD, err)
	}

	if _, err := os.Stat(m.SourcesList); err == nil {
		bak := fmt.Sprintf("%s.bak.%s", m.SourcesList, ts)
		if err := copyFile(m.SourcesList, bak); err != nil {
			return "", fmt.Errorf("backup %s: %w", m.SourcesList, err)
		}
		notes = append(notes, "backed up "+m.SourcesList+" → "+filepath.Base(bak))
	}
	content := "# Cleared by debian-network-tui\n# Use \"Apply apt sources from file\" to install new sources.\n"
	if err := os.WriteFile(m.SourcesList, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", m.SourcesList, err)
	}
	notes = append(notes, "cleared "+m.SourcesList)

	entries, err := os.ReadDir(m.SourcesListD)
	if err != nil && !os.IsNotExist(err) {
		return strings.Join(notes, "\n"), fmt.Errorf("read %s: %w", m.SourcesListD, err)
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".list") && !strings.HasSuffix(lower, ".sources") {
			continue
		}
		src := filepath.Join(m.SourcesListD, name)
		bak := filepath.Join(m.SourcesListD, name+".bak."+ts)
		if err := copyFile(src, bak); err != nil {
			return strings.Join(notes, "\n"), fmt.Errorf("backup %s: %w", src, err)
		}
		if err := os.Remove(src); err != nil {
			return strings.Join(notes, "\n"), fmt.Errorf("remove %s: %w", src, err)
		}
		removed++
	}
	notes = append(notes, fmt.Sprintf("removed %d drop-in(s) from %s", removed, m.SourcesListD))
	return strings.Join(notes, "\n"), nil
}

// Apply installs local config files into the system apt sources paths.
// Primary files replace SourcesList; others are copied into SourcesListD.
func (m Manager) Apply(cfgs []LocalConfig) (string, error) {
	if len(cfgs) == 0 {
		return "", fmt.Errorf("no local apt source files to apply")
	}
	if err := os.MkdirAll(m.SourcesListD, 0o755); err != nil {
		return "", fmt.Errorf("ensure %s: %w", m.SourcesListD, err)
	}

	ts := time.Now().Format("20060102-150405")
	var notes []string
	for _, c := range cfgs {
		data, err := os.ReadFile(c.Path)
		if err != nil {
			return strings.Join(notes, "\n"), fmt.Errorf("read %s: %w", c.Path, err)
		}
		var dest string
		if c.IsPrimary {
			dest = m.SourcesList
		} else {
			dest = filepath.Join(m.SourcesListD, c.TargetRel)
		}
		if _, err := os.Stat(dest); err == nil {
			bak := dest + ".bak." + ts
			if err := copyFile(dest, bak); err != nil {
				return strings.Join(notes, "\n"), fmt.Errorf("backup %s: %w", dest, err)
			}
			notes = append(notes, "backed up "+dest)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return strings.Join(notes, "\n"), fmt.Errorf("write %s: %w", dest, err)
		}
		notes = append(notes, fmt.Sprintf("installed %s → %s", filepath.Base(c.Path), dest))
	}
	return strings.Join(notes, "\n"), nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
