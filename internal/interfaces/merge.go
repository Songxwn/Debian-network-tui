package interfaces

// MergeWithDevices appends live system NICs that are not yet in the config
// so the UI can show every physical/virtual interface, not only configured ones.
func MergeWithDevices(conns []*Connection, names []string) []*Connection {
	seen := make(map[string]bool, len(conns))
	for _, c := range conns {
		if c != nil {
			seen[c.Name] = true
		}
	}
	out := append([]*Connection{}, conns...)
	for _, name := range names {
		if name == "" || name == "lo" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, &Connection{
			Name:       name,
			FromSystem: true,
		})
	}
	return out
}
