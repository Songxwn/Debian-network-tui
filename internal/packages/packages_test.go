package packages_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/debian-network-tui/debian-network-tui/internal/packages"
)

func TestFindBondVLANDebs(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"ifenslave_2.13_amd64.deb",
		"vlan_2.0.5_amd64.deb",
		"net-tools_2.10-0.1_amd64.deb",
		"other_1.0_all.deb",
		"readme.txt",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := packages.FindBondVLANDebs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ifenslave) != 1 || filepath.Base(res.Ifenslave[0]) != "ifenslave_2.13_amd64.deb" {
		t.Fatalf("ifenslave=%v", res.Ifenslave)
	}
	if len(res.VLAN) != 1 || filepath.Base(res.VLAN[0]) != "vlan_2.0.5_amd64.deb" {
		t.Fatalf("vlan=%v", res.VLAN)
	}
	if len(res.NetTools) != 1 || filepath.Base(res.NetTools[0]) != "net-tools_2.10-0.1_amd64.deb" {
		t.Fatalf("net-tools=%v", res.NetTools)
	}
	if len(res.Found()) != 3 {
		t.Fatalf("found=%v", res.Found())
	}
	if miss := res.MissingRequired(); len(miss) != 0 {
		t.Fatalf("unexpected missing: %v", miss)
	}
}
