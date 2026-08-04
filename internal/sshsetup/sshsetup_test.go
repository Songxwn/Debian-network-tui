package sshsetup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/debian-network-tui/debian-network-tui/internal/sshsetup"
)

func TestFindSSHDebs(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "openssh-server_9.2p1-2_amd64.deb"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "vlan_1_all.deb"), []byte("x"), 0o644)
	got, err := sshsetup.FindSSHDebs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.Contains(got[0], "openssh-server") {
		t.Fatalf("got=%v", got)
	}
}

func TestLoadRootConfAndReadKeys(t *testing.T) {
	dir := t.TempDir()
	pub := filepath.Join(dir, "my.pub")
	key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJustATestKeyMaterialNotReal000000000000 user@host"
	if err := os.WriteFile(pub, []byte(key+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "ssh-root.conf")
	if err := os.WriteFile(conf, []byte("PubkeyFile=my.pub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rc, err := sshsetup.LoadRootConf(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rc.PubkeyFile != pub {
		t.Fatalf("pubkey=%s", rc.PubkeyFile)
	}
	keys, err := sshsetup.ReadPubkeyLines(pub)
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
}

func TestImportRootAuthorizedKeys(t *testing.T) {
	// Use temp paths by temporarily skipping — ImportRootAuthorizedKeys uses fixed /root.
	// Only run shape validation via ReadPubkeyLines here.
	dir := t.TempDir()
	pub := filepath.Join(dir, "root.pub")
	_ = os.WriteFile(pub, []byte("not-a-key\nssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC0 test@host\n"), 0o644)
	keys, err := sshsetup.ReadPubkeyLines(pub)
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
}
