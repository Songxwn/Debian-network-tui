package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/debian-network-tui/debian-network-tui/internal/aptsources"
	"github.com/debian-network-tui/debian-network-tui/internal/packages"
	"github.com/debian-network-tui/debian-network-tui/internal/resolvconf"
	"github.com/debian-network-tui/debian-network-tui/internal/sshsetup"
)

const (
	StepDNS = iota
	StepClearApt
	StepApplyApt
	StepDebs
	StepSSH
	StepCount
)

// Plan holds resolved local files for a one-shot setup run.
type Plan struct {
	Dir string

	DNSSrc  string
	DNSDest string

	AptCfgs []aptsources.LocalConfig

	Debs []string

	SSHPubkey string
	SSHDebs   []string
	SSHMethod string
}

// Prepare scans dir and builds a plan. Returns an error if any required
// local file is missing (clear-apt needs no local file).
func Prepare(dir string) (*Plan, error) {
	if dir == "" {
		return nil, fmt.Errorf("directory is empty")
	}
	p := &Plan{Dir: dir}

	dnsFiles, err := resolvconf.FindLocalConfigs(dir)
	if err != nil {
		return nil, fmt.Errorf("find DNS config: %w", err)
	}
	if len(dnsFiles) == 0 {
		return nil, fmt.Errorf("missing DNS config (resolv.conf / dns.conf / dns-resolv.conf) in %s", dir)
	}
	p.DNSSrc = dnsFiles[0]
	p.DNSDest = resolvconf.DefaultPath
	if v := os.Getenv("RESOLV_CONF"); v != "" {
		p.DNSDest = v
	}

	aptCfgs, err := aptsources.FindLocalConfigs(dir)
	if err != nil {
		return nil, fmt.Errorf("find apt sources: %w", err)
	}
	if len(aptCfgs) == 0 {
		return nil, fmt.Errorf("missing apt source files (sources.list / *.list / *.sources) in %s", dir)
	}
	p.AptCfgs = aptCfgs

	found, err := packages.FindBondVLANDebs(dir)
	if err != nil {
		return nil, fmt.Errorf("find bond/vlan/net-tools debs: %w", err)
	}
	if miss := found.MissingRequired(); len(miss) > 0 {
		return nil, fmt.Errorf("missing packages in %s: %s", dir, strings.Join(miss, ", "))
	}
	p.Debs = found.Found()

	rc, err := sshsetup.LoadRootConf(dir)
	if err != nil {
		return nil, fmt.Errorf("SSH pubkey: %w", err)
	}
	p.SSHPubkey = rc.PubkeyFile
	bundle, err := sshsetup.FindSSHDebBundle(dir)
	if err != nil {
		return nil, fmt.Errorf("find SSH debs: %w", err)
	}
	if bundle.HasServer() {
		if miss := bundle.MissingDeps(); len(miss) > 0 {
			return nil, fmt.Errorf("SSH local debs incomplete in %s:\n  %s", dir, strings.Join(miss, "\n  "))
		}
		p.SSHDebs = bundle.InstallOrder()
		p.SSHMethod = "local .deb (server + deps)"
	} else {
		p.SSHDebs = nil
		p.SSHMethod = "apt-get install openssh-server"
	}

	return p, nil
}

// SummaryLines returns a human-readable preview for the confirm dialog.
func (p *Plan) SummaryLines() []string {
	if p == nil {
		return nil
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("Dir: %s", p.Dir))
	lines = append(lines, fmt.Sprintf("1) DNS: %s → %s", filepath.Base(p.DNSSrc), p.DNSDest))
	lines = append(lines, "2) Clear apt sources")
	var aptParts []string
	for _, c := range p.AptCfgs {
		dest := "/etc/apt/sources.list"
		if !c.IsPrimary {
			dest = "/etc/apt/sources.list.d/" + c.TargetRel
		}
		aptParts = append(aptParts, filepath.Base(c.Path)+"→"+filepath.Base(dest))
	}
	lines = append(lines, "3) Apply apt: "+strings.Join(aptParts, ", "))
	var debNames []string
	for _, d := range p.Debs {
		debNames = append(debNames, filepath.Base(d))
	}
	lines = append(lines, "4) Install: "+strings.Join(debNames, ", "))
	lines = append(lines, fmt.Sprintf("5) SSH: %s (%s)", filepath.Base(p.SSHPubkey), p.SSHMethod))
	return lines
}

// ExecuteStep runs a single step and returns log lines.
// On success, nextStep is step+1 (or StepCount when finished).
func (p *Plan) ExecuteStep(step int) (lines []string, nextStep int, err error) {
	if p == nil {
		return []string{"plan is nil"}, step, fmt.Errorf("plan is nil")
	}
	nextStep = step + 1
	switch step {
	case StepDNS:
		lines = append(lines, fmt.Sprintf("==> [1/%d] Overwrite DNS", StepCount))
		lines = append(lines, fmt.Sprintf("    %s → %s", p.DNSSrc, p.DNSDest))
		bak, e := resolvconf.OverwriteFromFile(p.DNSSrc, p.DNSDest)
		if e != nil {
			lines = append(lines, "    FAIL: "+e.Error())
			return lines, step, e
		}
		if bak != "" {
			lines = append(lines, "    backup: "+bak)
		}
		lines = append(lines, "    OK")
		return lines, nextStep, nil

	case StepClearApt:
		lines = append(lines, fmt.Sprintf("==> [2/%d] Clear apt sources", StepCount))
		note, e := aptsources.Default().Clear()
		for _, ln := range splitNote(note) {
			lines = append(lines, "    "+ln)
		}
		if e != nil {
			lines = append(lines, "    FAIL: "+e.Error())
			return lines, step, e
		}
		lines = append(lines, "    OK")
		return lines, nextStep, nil

	case StepApplyApt:
		lines = append(lines, fmt.Sprintf("==> [3/%d] Apply apt sources from file", StepCount))
		note, e := aptsources.Default().Apply(p.AptCfgs)
		for _, ln := range splitNote(note) {
			lines = append(lines, "    "+ln)
		}
		if e != nil {
			lines = append(lines, "    FAIL: "+e.Error())
			return lines, step, e
		}
		lines = append(lines, "    OK")
		return lines, nextStep, nil

	case StepDebs:
		lines = append(lines, fmt.Sprintf("==> [4/%d] Install ifenslave/vlan/net-tools packages", StepCount))
		for _, d := range p.Debs {
			lines = append(lines, "    "+filepath.Base(d))
		}
		msg, e := packages.InstallLocalDebs(p.Debs)
		for _, ln := range indentBlock(msg, 80) {
			lines = append(lines, "    | "+ln)
		}
		if e != nil {
			lines = append(lines, "    FAIL: "+e.Error())
			return lines, step, e
		}
		lines = append(lines, "    OK")
		return lines, nextStep, nil

	case StepSSH:
		lines = append(lines, fmt.Sprintf("==> [5/%d] Configure SSH (root key)", StepCount))
		lines = append(lines, "    method: "+p.SSHMethod)
		lines = append(lines, "    pubkey: "+p.SSHPubkey)
		res, e := sshsetup.Run(p.Dir, packages.InstallLocalDebs)
		if res != nil {
			if res.InstallDetail != "" {
				for _, ln := range indentBlock(res.InstallDetail, 60) {
					lines = append(lines, "    | "+ln)
				}
			}
			lines = append(lines, fmt.Sprintf("    install: %s", res.InstallMethod))
			lines = append(lines, fmt.Sprintf("    keys: %d new / %d total", res.KeysAdded, res.KeysTotal))
			lines = append(lines, "    sshd drop-in: "+res.SSHDDropIn)
		}
		if e != nil {
			lines = append(lines, "    FAIL: "+e.Error())
			return lines, step, e
		}
		lines = append(lines, "    OK")
		lines = append(lines, "")
		lines = append(lines, "All steps completed successfully.")
		return lines, StepCount, nil
	}
	return []string{fmt.Sprintf("unknown step %d", step)}, step, fmt.Errorf("unknown step %d", step)
}

func splitNote(note string) []string {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	return strings.Split(note, "\n")
}

func indentBlock(text string, maxLines int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	raw := strings.Split(text, "\n")
	if maxLines > 0 && len(raw) > maxLines {
		raw = raw[len(raw)-maxLines:]
		raw = append([]string{"..."}, raw...)
	}
	return raw
}
