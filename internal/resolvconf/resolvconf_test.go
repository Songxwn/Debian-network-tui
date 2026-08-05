package resolvconf_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/debian-network-tui/debian-network-tui/internal/resolvconf"
)

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	src := `# test
nameserver 8.8.8.8
nameserver 1.1.1.1
search example.com local
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := resolvconf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Nameservers) != 2 || c.Nameservers[0] != "8.8.8.8" {
		t.Fatalf("nameservers=%v", c.Nameservers)
	}
	if len(c.Search) != 2 {
		t.Fatalf("search=%v", c.Search)
	}
	c.Nameservers = []string{"9.9.9.9", "8.8.4.4"}
	c.Search = []string{"corp.example"}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"nameserver 9.9.9.9", "nameserver 8.8.4.4", "search corp.example"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if _, err := os.Stat(path + ".bak."); err == nil {
		// bak has timestamp suffix — just ensure some bak exists
	}
	matches, _ := filepath.Glob(path + ".bak.*")
	if len(matches) == 0 {
		t.Fatal("expected backup file")
	}
}

func TestOverwriteFromFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "resolv.conf")
	dest := filepath.Join(dir, "etc-resolv.conf")
	content := "nameserver 9.9.9.9\nnameserver 149.112.112.112\n# custom\noptions edns0\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("nameserver 1.1.1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bak, err := resolvconf.OverwriteFromFile(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if bak == "" {
		t.Fatal("expected backup path")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("dest content mismatch:\n%s", got)
	}
	old, err := os.ReadFile(bak)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(old), "1.1.1.1") {
		t.Fatalf("backup wrong: %s", old)
	}
}

func TestFindLocalConfigs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dns.conf")
	if err := os.WriteFile(path, []byte("nameserver 8.8.8.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := resolvconf.FindLocalConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != path {
		t.Fatalf("found=%v", found)
	}
}

func TestValidateRejectsBadIP(t *testing.T) {
	c := &resolvconf.Config{Nameservers: []string{"not-an-ip"}}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}
