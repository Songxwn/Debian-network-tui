package interfaces

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load reads path and recursively resolves source / source-directory.
func Load(path string) (*File, error) {
	return load(path, map[string]bool{})
}

func load(path string, seen map[string]bool) (*File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if seen[abs] {
		return &File{Path: path}, nil
	}
	seen[abs] = true

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	f := &File{Path: path}
	f.Stanzas, err = parseContent(string(data))
	if err != nil {
		return nil, err
	}

	for _, s := range f.Stanzas {
		switch s.Kind {
		case KindSource:
			pattern := resolveSourcePath(path, s.Path)
			matches, err := filepath.Glob(pattern)
			if err != nil || len(matches) == 0 {
				// Try literal path when glob finds nothing (or no meta chars).
				if _, statErr := os.Stat(pattern); statErr == nil {
					matches = []string{pattern}
				}
			}
			for _, mp := range matches {
				info, err := os.Stat(mp)
				if err != nil || info.IsDir() {
					continue
				}
				sf, err := load(mp, seen)
				if err != nil {
					continue
				}
				f.Sources = append(f.Sources, sf)
			}
		case KindSourceDirectory:
			dir := resolveSourcePath(path, s.Path)
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if strings.HasPrefix(name, ".") {
					continue
				}
				sf, err := load(filepath.Join(dir, name), seen)
				if err != nil {
					continue
				}
				f.Sources = append(f.Sources, sf)
			}
		}
	}
	return f, nil
}

func resolveSourcePath(parent, src string) string {
	if filepath.IsAbs(src) {
		return src
	}
	return filepath.Join(filepath.Dir(parent), src)
}

func parseContent(content string) ([]Stanza, error) {
	var stanzas []Stanza
	scanner := bufio.NewScanner(strings.NewReader(content))
	var current *Stanza

	flush := func() {
		if current != nil {
			stanzas = append(stanzas, *current)
			current = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			flush()
			stanzas = append(stanzas, Stanza{Kind: KindBlank, Raw: line})
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			flush()
			stanzas = append(stanzas, Stanza{Kind: KindComment, Raw: line})
			continue
		}

		// Continuation / option under iface or mapping.
		if current != nil && (current.Kind == KindIface || current.Kind == KindMapping) &&
			(strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			key, val, ok := splitOption(trimmed)
			if ok && current.Iface != nil {
				current.Iface.Options = append(current.Iface.Options, Option{Key: key, Value: val})
			} else if ok {
				// mapping without iface pointer — store as raw
				current.Raw += "\n" + line
			}
			continue
		}

		flush()
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "auto":
			stanzas = append(stanzas, Stanza{Kind: KindAuto, Names: fields[1:], Raw: line})
		case "allow-hotplug":
			stanzas = append(stanzas, Stanza{Kind: KindAllowHotplug, Names: fields[1:], Raw: line})
		case "iface":
			if len(fields) < 4 {
				stanzas = append(stanzas, Stanza{Kind: KindOther, Raw: line})
				continue
			}
			iface := &Iface{
				Name:   fields[1],
				Family: Family(fields[2]),
				Method: Method(fields[3]),
			}
			current = &Stanza{Kind: KindIface, Iface: iface, Raw: line}
		case "mapping":
			current = &Stanza{Kind: KindMapping, Raw: line}
		case "source":
			if len(fields) >= 2 {
				stanzas = append(stanzas, Stanza{Kind: KindSource, Path: fields[1], Raw: line})
			} else {
				stanzas = append(stanzas, Stanza{Kind: KindOther, Raw: line})
			}
		case "source-directory":
			if len(fields) >= 2 {
				stanzas = append(stanzas, Stanza{Kind: KindSourceDirectory, Path: fields[1], Raw: line})
			} else {
				stanzas = append(stanzas, Stanza{Kind: KindOther, Raw: line})
			}
		default:
			if strings.HasPrefix(fields[0], "allow-") {
				stanzas = append(stanzas, Stanza{
					Kind:  KindAllow,
					Allow: strings.TrimPrefix(fields[0], "allow-"),
					Names: fields[1:],
					Raw:   line,
				})
			} else {
				stanzas = append(stanzas, Stanza{Kind: KindOther, Raw: line})
			}
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return stanzas, nil
}

func splitOption(line string) (key, value string, ok bool) {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	key = parts[0]
	if len(parts) == 2 {
		value = strings.TrimSpace(parts[1])
	}
	return key, value, true
}

// Connections flattens this file and sourced files into named connections.
func (f *File) Connections() []*Connection {
	byName := map[string]*Connection{}
	order := []string{}

	var walk func(*File)
	walk = func(file *File) {
		if file == nil {
			return
		}
		for _, s := range file.Stanzas {
			switch s.Kind {
			case KindAuto:
				for _, n := range s.Names {
					c := ensure(byName, &order, n)
					c.Auto = true
				}
			case KindAllowHotplug:
				for _, n := range s.Names {
					c := ensure(byName, &order, n)
					c.AllowHotplug = true
				}
			case KindIface:
				if s.Iface == nil {
					continue
				}
				c := ensure(byName, &order, s.Iface.Name)
				switch s.Iface.Family {
				case FamilyInet:
					cp := *s.Iface
					c.IPv4 = &cp
				case FamilyInet6:
					cp := *s.Iface
					c.IPv6 = &cp
				default:
					cp := *s.Iface
					c.Extra = append(c.Extra, &cp)
				}
			}
		}
		for _, src := range file.Sources {
			walk(src)
		}
	}
	walk(f)

	out := make([]*Connection, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out
}

func ensure(m map[string]*Connection, order *[]string, name string) *Connection {
	if c, ok := m[name]; ok {
		return c
	}
	c := &Connection{Name: name}
	m[name] = c
	*order = append(*order, name)
	return c
}

// FindConnection returns a connection by name.
func (f *File) FindConnection(name string) *Connection {
	for _, c := range f.Connections() {
		if c.Name == name {
			return c
		}
	}
	return nil
}
