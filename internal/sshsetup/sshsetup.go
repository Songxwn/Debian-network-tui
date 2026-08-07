package sshsetup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	defaultPubkeyName = "root.pub"
	defaultConfName   = "ssh-root.conf"
	sshdDropInPath    = "/etc/ssh/sshd_config.d/99-debian-network-tui-rootkey.conf"
	rootSSHDir        = "/root/.ssh"
	rootAuthKeys      = "/root/.ssh/authorized_keys"
)

// RootConf describes how to locate the root public key.
type RootConf struct {
	Dir        string
	ConfPath   string // ssh-root.conf if present
	PubkeyFile string // absolute path to pubkey file
}

// FindSSHDebs returns openssh-server and its common local dependency .deb paths in dir.
// Dependency packages (matched by filename):
//
//	openssh-client, openssh-sftp-server, runit-helper, libssl3*, libwrap0
//
// Order is dependencies first, then openssh-server, so apt can satisfy them together.
func FindSSHDebs(dir string) ([]string, error) {
	bundle, err := FindSSHDebBundle(dir)
	if err != nil {
		return nil, err
	}
	return bundle.InstallOrder(), nil
}

// SSHDebBundle classifies local SSH-related .deb packages.
type SSHDebBundle struct {
	Dir        string
	Server     []string
	Client     []string
	SFTP       []string
	RunitHelp  []string
	LibSSL3    []string
	LibWrap0   []string
}

// InstallOrder returns dependency debs first, then openssh-server.
func (b SSHDebBundle) InstallOrder() []string {
	var out []string
	out = append(out, b.Client...)
	out = append(out, b.SFTP...)
	out = append(out, b.RunitHelp...)
	out = append(out, b.LibSSL3...)
	out = append(out, b.LibWrap0...)
	out = append(out, b.Server...)
	return out
}

// HasServer reports whether an openssh-server .deb was found.
func (b SSHDebBundle) HasServer() bool {
	return len(b.Server) > 0
}

// MissingDeps returns required dependency patterns missing when a local server deb is used.
func (b SSHDebBundle) MissingDeps() []string {
	var miss []string
	if len(b.Client) == 0 {
		miss = append(miss, "openssh-client_*.deb")
	}
	if len(b.SFTP) == 0 {
		miss = append(miss, "openssh-sftp-server_*.deb")
	}
	if len(b.RunitHelp) == 0 {
		miss = append(miss, "runit-helper_*.deb")
	}
	if len(b.LibSSL3) == 0 {
		miss = append(miss, "libssl3_*.deb")
	}
	if len(b.LibWrap0) == 0 {
		miss = append(miss, "libwrap0_*.deb")
	}
	return miss
}

// FindSSHDebBundle scans dir for openssh-server and its local dependency debs.
func FindSSHDebBundle(dir string) (SSHDebBundle, error) {
	b := SSHDebBundle{Dir: dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return b, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".deb") {
			continue
		}
		abs := filepath.Join(dir, name)
		switch {
		case strings.Contains(lower, "openssh-sftp-server"):
			b.SFTP = append(b.SFTP, abs)
		case strings.Contains(lower, "openssh-client"):
			b.Client = append(b.Client, abs)
		case strings.Contains(lower, "openssh-server") ||
			(strings.Contains(lower, "openssh") && strings.Contains(lower, "server") &&
				!strings.Contains(lower, "client") && !strings.Contains(lower, "sftp")):
			b.Server = append(b.Server, abs)
		case strings.Contains(lower, "runit-helper"):
			b.RunitHelp = append(b.RunitHelp, abs)
		case strings.Contains(lower, "libssl3"):
			b.LibSSL3 = append(b.LibSSL3, abs)
		case strings.Contains(lower, "libwrap0"):
			b.LibWrap0 = append(b.LibWrap0, abs)
		}
	}
	return b, nil
}

// LoadRootConf reads ssh-root.conf (optional) and resolves the pubkey path.
// Conf keys (case-insensitive):
//
//	PubkeyFile=root.pub
//
// Default pubkey candidates beside the binary: root.pub, id_rsa.pub, id_ed25519.pub
func LoadRootConf(dir string) (*RootConf, error) {
	rc := &RootConf{Dir: dir}
	confPath := filepath.Join(dir, defaultConfName)
	pubkeyRel := ""

	if data, err := os.ReadFile(confPath); err == nil {
		rc.ConfPath = confPath
		sc := bufio.NewScanner(strings.NewReader(string(data)))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(strings.ToLower(key))
			val = strings.TrimSpace(val)
			val = strings.Trim(val, `"'`)
			if key == "pubkeyfile" || key == "pubkey" || key == "publickeyfile" {
				pubkeyRel = val
			}
		}
	}

	candidates := []string{}
	if pubkeyRel != "" {
		if filepath.IsAbs(pubkeyRel) {
			candidates = append(candidates, pubkeyRel)
		} else {
			candidates = append(candidates, filepath.Join(dir, pubkeyRel))
		}
	}
	candidates = append(candidates,
		filepath.Join(dir, defaultPubkeyName),
		filepath.Join(dir, "id_rsa.pub"),
		filepath.Join(dir, "id_ed25519.pub"),
		filepath.Join(dir, "authorized_keys"),
	)

	seen := map[string]bool{}
	for _, p := range candidates {
		if seen[p] {
			continue
		}
		seen[p] = true
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			rc.PubkeyFile = p
			return rc, nil
		}
	}
	return rc, fmt.Errorf("public key file not found in %s (set PubkeyFile in %s or place %s)", dir, defaultConfName, defaultPubkeyName)
}

// ReadPubkeyLines returns non-empty, non-comment pubkey lines from path.
func ReadPubkeyLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var keys []string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Basic ssh public key shape: type + key material
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "ssh-rsa", "ssh-ed25519", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384",
			"ecdsa-sha2-nistp521", "ssh-dss", "sk-ssh-ed25519@openssh.com",
			"sk-ecdsa-sha2-nistp256@openssh.com":
			keys = append(keys, line)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no valid SSH public keys in %s", path)
	}
	return keys, nil
}

// EnsureOpenSSHInstalled installs openssh-server (and local deps) from .deb or apt.
// When a local openssh-server*.deb is present, companion dependency debs in the same
// directory are installed together (openssh-client, openssh-sftp-server, runit-helper,
// libssl3, libwrap0). Missing required deps return an error before apt runs.
func EnsureOpenSSHInstalled(dir string, installDebs func([]string) (string, error), aptInstall func(string) (string, error)) (method string, detail string, err error) {
	bundle, err := FindSSHDebBundle(dir)
	if err != nil {
		return "", "", err
	}
	if bundle.HasServer() {
		if miss := bundle.MissingDeps(); len(miss) > 0 {
			return "local-deb", "", fmt.Errorf(
				"openssh-server .deb found, but missing dependency packages in %s:\n  %s\nPlace matching .deb files next to openssh-server (same versions as the server package)",
				dir, strings.Join(miss, "\n  "))
		}
		msg, err := installDebs(bundle.InstallOrder())
		return "local-deb", msg, err
	}
	msg, err := aptInstall("openssh-server")
	return "apt", msg, err
}

// ConfigureSSHD writes a drop-in enabling root key-based login.
func ConfigureSSHD() error {
	dir := filepath.Dir(sshdDropInPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	content := `# Managed by debian-network-tui
PermitRootLogin prohibit-password
PubkeyAuthentication yes
AuthorizedKeysFile .ssh/authorized_keys
`
	if err := os.WriteFile(sshdDropInPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", sshdDropInPath, err)
	}
	return nil
}

// ImportRootAuthorizedKeys appends missing pubkey lines into root's authorized_keys.
func ImportRootAuthorizedKeys(keys []string) (added int, err error) {
	if err := os.MkdirAll(rootSSHDir, 0o700); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", rootSSHDir, err)
	}
	_ = os.Chmod(rootSSHDir, 0o700)

	existing := map[string]bool{}
	if data, err := os.ReadFile(rootAuthKeys); err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(data)))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				existing[line] = true
			}
		}
	}

	var toAppend []string
	for _, k := range keys {
		if !existing[k] {
			toAppend = append(toAppend, k)
			existing[k] = true
		}
	}
	if len(toAppend) == 0 {
		_ = os.Chmod(rootAuthKeys, 0o600)
		return 0, nil
	}

	f, err := os.OpenFile(rootAuthKeys, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", rootAuthKeys, err)
	}
	defer f.Close()
	for _, k := range toAppend {
		if _, err := f.WriteString(k + "\n"); err != nil {
			return 0, err
		}
	}
	_ = os.Chmod(rootAuthKeys, 0o600)
	return len(toAppend), nil
}

// RestartSSH reloads/restarts the OpenSSH service.
func RestartSSH() error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		// Debian unit name is "ssh"
		cmd := exec.Command("systemctl", "restart", "ssh")
		out, err := cmd.CombinedOutput()
		if err != nil {
			// fallback openssh
			cmd2 := exec.Command("systemctl", "restart", "sshd")
			out2, err2 := cmd2.CombinedOutput()
			if err2 != nil {
				return fmt.Errorf("systemctl restart ssh failed: %s / %s",
					strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
			}
		}
		return nil
	}
	cmd := exec.Command("service", "ssh", "restart")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("service ssh restart failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// AptInstallPackage runs apt-get install -y <pkg>.
func AptInstallPackage(pkg string) (string, error) {
	apt, err := exec.LookPath("apt-get")
	if err != nil {
		apt, err = exec.LookPath("apt")
		if err != nil {
			return "", fmt.Errorf("apt-get/apt not found")
		}
	}
	cmd := exec.Command(apt, "install", "-y", pkg)
	cmd.Env = append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"NEEDRESTART_MODE=a",
	)
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			return "", fmt.Errorf("apt install %s failed: %w", pkg, err)
		}
		return msg, fmt.Errorf("apt install %s failed: %s", pkg, msg)
	}
	if msg == "" {
		msg = "installed " + pkg
	}
	return msg, nil
}

// SetupResult summarizes the SSH setup operation.
type SetupResult struct {
	InstallMethod string
	InstallDetail string
	PubkeyFile    string
	KeysAdded     int
	KeysTotal     int
	SSHDDropIn    string
}

// Run performs install + sshd config + root key import + restart.
func Run(dir string, installDebs func([]string) (string, error)) (*SetupResult, error) {
	res := &SetupResult{SSHDDropIn: sshdDropInPath}

	method, detail, err := EnsureOpenSSHInstalled(dir, installDebs, AptInstallPackage)
	if err != nil {
		return res, fmt.Errorf("install openssh-server: %w", err)
	}
	res.InstallMethod = method
	res.InstallDetail = detail

	rc, err := LoadRootConf(dir)
	if err != nil {
		return res, err
	}
	res.PubkeyFile = rc.PubkeyFile

	keys, err := ReadPubkeyLines(rc.PubkeyFile)
	if err != nil {
		return res, err
	}
	res.KeysTotal = len(keys)

	if err := ConfigureSSHD(); err != nil {
		return res, err
	}

	added, err := ImportRootAuthorizedKeys(keys)
	if err != nil {
		return res, err
	}
	res.KeysAdded = added

	if err := RestartSSH(); err != nil {
		return res, fmt.Errorf("ssh configured but restart failed: %w", err)
	}
	return res, nil
}
