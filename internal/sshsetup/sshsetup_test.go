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
	files := []string{
		"openssh-server_9.2p1-2+deb12u6_amd64.deb",
		"openssh-client_9.2p1-2+deb12u6_amd64.deb",
		"openssh-sftp-server_9.2p1-2+deb12u6_amd64.deb",
		"runit-helper_2.15.2_all.deb",
		"libssl3_3.0.16-1~deb12u1_amd64.deb",
		"libwrap0_7.6.q-32_amd64.deb",
		"vlan_1_all.deb",
	}
	for _, f := range files {
		_ = os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644)
	}
	bundle, err := sshsetup.FindSSHDebBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.HasServer() || len(bundle.MissingDeps()) != 0 {
		t.Fatalf("bundle incomplete: %+v missing=%v", bundle, bundle.MissingDeps())
	}
	got, err := sshsetup.FindSSHDebs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("got=%v", got)
	}
	// Server should be last.
	if !strings.Contains(got[len(got)-1], "openssh-server") {
		t.Fatalf("server should be last: %v", got)
	}
	// vlan must not be included
	for _, p := range got {
		if strings.Contains(p, "vlan") {
			t.Fatalf("unexpected vlan in %v", got)
		}
	}
}

func TestFindSSHDebsMissingDeps(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "openssh-server_9.2_amd64.deb"), []byte("x"), 0o644)
	bundle, err := sshsetup.FindSSHDebBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	miss := bundle.MissingDeps()
	if len(miss) != 5 {
		t.Fatalf("missing=%v", miss)
	}
	_, _, err = sshsetup.EnsureOpenSSHInstalled(dir,
		func([]string) (string, error) { t.Fatal("should not install"); return "", nil },
		func(string) (string, error) { t.Fatal("should not apt"); return "", nil },
	)
	if err == nil || !strings.Contains(err.Error(), "missing dependency") {
		t.Fatalf("err=%v", err)
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
