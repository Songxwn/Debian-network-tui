package interfaces_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/debian-network-tui/debian-network-tui/internal/interfaces"
)

const sample = `# interfaces(5)
source /etc/network/interfaces.d/*

auto lo
iface lo inet loopback

allow-hotplug eth0
iface eth0 inet static
    address 192.168.1.10
    netmask 255.255.255.0
    gateway 192.168.1.1
    dns-nameservers 8.8.8.8 1.1.1.1

iface eth0 inet6 static
    address 2001:db8::10/64
    gateway 2001:db8::1
`

func TestParseConnections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "interfaces")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	// create empty interfaces.d so source does not fail loudly
	_ = os.MkdirAll(filepath.Join(dir, "interfaces.d"), 0o755)

	// Fix source path in sample for relative test — rewrite file with local source
	content := strings.Replace(sample, "source /etc/network/interfaces.d/*", "source-directory interfaces.d", 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := interfaces.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	conns := f.Connections()
	if len(conns) < 2 {
		t.Fatalf("expected >=2 connections, got %d", len(conns))
	}

	var eth *interfaces.Connection
	for _, c := range conns {
		if c.Name == "eth0" {
			eth = c
		}
	}
	if eth == nil {
		t.Fatal("eth0 not found")
	}
	if !eth.AllowHotplug {
		t.Error("expected allow-hotplug")
	}
	if eth.IPv4 == nil || eth.IPv4.Method != interfaces.MethodStatic {
		t.Fatalf("bad ipv4: %+v", eth.IPv4)
	}
	if eth.IPv4.GetOption("address") != "192.168.1.10" {
		t.Errorf("address=%q", eth.IPv4.GetOption("address"))
	}
	if eth.IPv6 == nil || eth.IPv6.GetOption("address") != "2001:db8::10/64" {
		t.Errorf("bad ipv6: %+v", eth.IPv6)
	}
}

func TestSaveAndDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "interfaces")
	base := `auto lo
iface lo inet loopback
`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := interfaces.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c := interfaces.NewConnection("ens18")
	c.Auto = true
	c.AllowHotplug = true
	c.EnsureIPv4(interfaces.MethodStatic)
	c.IPv4.SetOption("address", "10.0.0.5")
	c.IPv4.SetOption("netmask", "255.255.255.0")
	c.IPv4.SetOption("gateway", "10.0.0.1")

	if err := interfaces.ValidateConnection(c); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveConnection(c); err != nil {
		t.Fatal(err)
	}

	f2, err := interfaces.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := f2.FindConnection("ens18")
	if got == nil || got.IPv4.GetOption("address") != "10.0.0.5" {
		t.Fatalf("save failed: %+v", got)
	}

	if err := f2.DeleteConnection("ens18"); err != nil {
		t.Fatal(err)
	}
	f3, err := interfaces.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f3.FindConnection("ens18") != nil {
		t.Fatal("delete failed")
	}
	if f3.FindConnection("lo") == nil {
		t.Fatal("lo should remain")
	}
}

func TestValidateRejectsBadIP(t *testing.T) {
	c := interfaces.NewConnection("eth0")
	c.EnsureIPv4(interfaces.MethodStatic)
	c.IPv4.SetOption("address", "999.1.1.1")
	if err := interfaces.ValidateConnection(c); err == nil {
		t.Fatal("expected validation error")
	}
}
