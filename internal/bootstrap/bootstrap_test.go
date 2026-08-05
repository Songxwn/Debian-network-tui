package bootstrap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/debian-network-tui/debian-network-tui/internal/bootstrap"
)

func TestPrepareMissingFiles(t *testing.T) {
	dir := t.TempDir()
	_, err := bootstrap.Prepare(dir)
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
	if !strings.Contains(err.Error(), "DNS") {
		t.Fatalf("want DNS missing, got %v", err)
	}
}

func TestPrepareOK(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("resolv.conf", "nameserver 8.8.8.8\n")
	mustWrite("sources.list", "deb http://deb.debian.org/debian bookworm main\n")
	mustWrite("ifenslave_2.deb", "fake")
	mustWrite("vlan_1.deb", "fake")
	mustWrite("root.pub", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeKeyForUnitTest only-for-test\n")

	p, err := bootstrap.Prepare(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.DNSSrc == "" || len(p.AptCfgs) == 0 || len(p.Debs) != 2 {
		t.Fatalf("plan incomplete: %+v", p)
	}
	if p.SSHPubkey == "" {
		t.Fatal("expected SSH pubkey")
	}
	sum := strings.Join(p.SummaryLines(), "\n")
	for _, want := range []string{"DNS:", "Clear apt", "Apply apt", "Install:", "SSH:"} {
		if !strings.Contains(sum, want) {
			t.Fatalf("summary missing %q:\n%s", want, sum)
		}
	}
}
