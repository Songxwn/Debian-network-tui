package interfaces_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/debian-network-tui/debian-network-tui/internal/interfaces"
)

func TestMergeWithDevices(t *testing.T) {
	conns := []*interfaces.Connection{
		{Name: "lo"},
		{Name: "eth0", IPv4: &interfaces.Iface{Name: "eth0", Family: interfaces.FamilyInet, Method: interfaces.MethodDHCP}},
	}
	merged := interfaces.MergeWithDevices(conns, []string{"lo", "eth0", "eth1", "ens18"})
	names := map[string]bool{}
	for _, c := range merged {
		names[c.Name] = true
	}
	for _, want := range []string{"lo", "eth0", "eth1", "ens18"} {
		if !names[want] {
			t.Fatalf("missing %s in merged list", want)
		}
	}
	var eth1 *interfaces.Connection
	for _, c := range merged {
		if c.Name == "eth1" {
			eth1 = c
		}
	}
	if eth1 == nil || !eth1.FromSystem || eth1.Configured() {
		t.Fatalf("eth1 should be unconfigured system stub: %+v", eth1)
	}
}

func TestSaveBondAndVLAN(t *testing.T) {
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

	bond := &interfaces.Connection{
		Name: "bond0",
		Auto: true,
		IPv4: &interfaces.Iface{
			Name:   "bond0",
			Family: interfaces.FamilyInet,
			Method: interfaces.MethodManual,
			Options: []interfaces.Option{
				{Key: "bond-slaves", Value: "eth0 eth1"},
				{Key: "bond-mode", Value: "802.3ad"},
				{Key: "bond-miimon", Value: "100"},
				{Key: "bond-lacp-rate", Value: "fast"},
			},
		},
	}
	if err := interfaces.ValidateConnection(bond); err != nil {
		t.Fatal(err)
	}
	if bond.Type() != interfaces.TypeBond {
		t.Fatalf("type=%s", bond.Type())
	}
	if err := f.SaveConnection(bond); err != nil {
		t.Fatal(err)
	}

	f2, err := interfaces.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b := f2.FindConnection("bond0")
	if b == nil || b.BondMode() != "802.3ad" {
		t.Fatalf("bond missing: %+v", b)
	}
	s0 := f2.FindConnection("eth0")
	if s0 == nil || s0.IPv4 == nil || s0.IPv4.GetOption("bond-master") != "bond0" {
		t.Fatalf("slave eth0 missing bond-master: %+v", s0)
	}

	vlan := &interfaces.Connection{
		Name:         "bond0.100",
		AllowHotplug: true,
		IPv4: &interfaces.Iface{
			Name:   "bond0.100",
			Family: interfaces.FamilyInet,
			Method: interfaces.MethodStatic,
			Options: []interfaces.Option{
				{Key: "vlan-raw-device", Value: "bond0"},
				{Key: "vlan_id", Value: "100"},
				{Key: "address", Value: "10.10.10.2"},
				{Key: "netmask", Value: "255.255.255.0"},
				{Key: "gateway", Value: "10.10.10.1"},
			},
		},
	}
	if err := interfaces.ValidateConnection(vlan); err != nil {
		t.Fatal(err)
	}
	if vlan.Type() != interfaces.TypeVLAN {
		t.Fatalf("type=%s", vlan.Type())
	}
	if err := f2.SaveConnection(vlan); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"iface bond0 inet manual",
		"bond-slaves eth0 eth1",
		"iface eth0 inet manual",
		"bond-master bond0",
		"iface bond0.100 inet static",
		"vlan-raw-device bond0",
		"vlan_id 100",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered config missing %q\n%s", want, text)
		}
	}
}
