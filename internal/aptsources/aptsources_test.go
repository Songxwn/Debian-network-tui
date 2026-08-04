package aptsources_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/debian-network-tui/debian-network-tui/internal/aptsources"
)

func TestFindLocalConfigs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sources.list"), []byte("deb http://deb.debian.org/debian bookworm main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.list"), []byte("deb http://example/ extra main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sources.list.d")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "security.sources"), []byte("Types: deb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgs, err := aptsources.FindLocalConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgs) != 3 {
		t.Fatalf("got %d configs: %+v", len(cfgs), cfgs)
	}
	if !cfgs[0].IsPrimary || filepath.Base(cfgs[0].Path) != "sources.list" {
		t.Fatalf("primary=%+v", cfgs[0])
	}
}

func TestClearAndApply(t *testing.T) {
	root := t.TempDir()
	list := filepath.Join(root, "sources.list")
	listD := filepath.Join(root, "sources.list.d")
	if err := os.MkdirAll(listD, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(list, []byte("deb http://old/ debian main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldDrop := filepath.Join(listD, "old.list")
	if err := os.WriteFile(oldDrop, []byte("deb http://old-drop/ main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := aptsources.Manager{SourcesList: list, SourcesListD: listD}
	note, err := m.Clear()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, "cleared") {
		t.Fatalf("note=%q", note)
	}
	raw, _ := os.ReadFile(list)
	if strings.Contains(string(raw), "http://old/") {
		t.Fatalf("sources.list not cleared: %s", raw)
	}
	if _, err := os.Stat(oldDrop); !os.IsNotExist(err) {
		t.Fatal("old drop-in should be removed")
	}

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "sources.list")
	if err := os.WriteFile(srcFile, []byte("deb http://new/debian bookworm main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(srcDir, "mirror.list")
	if err := os.WriteFile(extra, []byte("deb http://mirror/ bookworm main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgs, err := aptsources.FindLocalConfigs(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	note, err = m.Apply(cfgs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(list)
	if !strings.Contains(string(raw), "http://new/debian") {
		t.Fatalf("primary not applied: %s", raw)
	}
	drop, _ := os.ReadFile(filepath.Join(listD, "mirror.list"))
	if !strings.Contains(string(drop), "http://mirror/") {
		t.Fatalf("drop-in not applied: %s (%s)", drop, note)
	}
}
