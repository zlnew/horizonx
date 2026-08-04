// Agent provisioning for the setup wizard.
//
// Ported from scripts/install-agent.sh into Go: creates the horizonx user,
// adds it to the docker group, generates an ed25519 SSH key for git access,
// and writes the agent env + systemd unit. Each step runs via sudo when the
// target path requires elevation.
package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AgentProvision holds everything needed to install the agent.
type AgentProvision struct {
	// Env values for the agent.
	APIURL      string // HORIZONX_API_URL
	WSURL       string // HORIZONX_WS_URL
	ServerID    string
	ServerToken string
	RedisAddr   string

	// User + paths (defaults: horizonx / /var/lib/horizonx / /etc/horizonx).
	UserName  string
	GroupName string
	DataDir   string
	EnvDir    string
}

// defaultAgentProvision fills user/path defaults.
func defaultAgentProvision() *AgentProvision {
	return &AgentProvision{
		UserName:  "horizonx",
		GroupName: "horizonx",
		DataDir:   "/var/lib/horizonx",
		EnvDir:    "/etc/horizonx",
	}
}

// provisionStep is one named step of agent provisioning.
type provisionStep struct {
	name string
	fn   func() error
}

// provisionSteps lists the agent provisioning steps in execution order.
func (p *AgentProvision) provisionSteps() []provisionStep {
	return []provisionStep{
		{"create system user", p.createUser},
		{"add docker group", p.addDockerGroup},
		{"create directories", p.createDirs},
		{"udev rules", p.writeUdevRules},
		{"generate SSH key", p.genSSHKey},
		{"write SSH config", p.writeSSHConfig},
		{"populate known_hosts", p.populateKnownHosts},
		{"write env file", p.writeEnvFile},
		{"install systemd unit", p.installSystemdUnit},
		{"enable service", p.enableService},
	}
}

// ProvisionAgent creates the user, docker group membership, udev rules, SSH
// key, env file, and systemd unit. Runs the privileged steps via sudo.
func ProvisionAgent(p *AgentProvision) error {
	steps := p.provisionSteps()
	for _, s := range steps {
		fmt.Printf("  • %s…\n", s.name)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return nil
}

func (p *AgentProvision) createUser() error {
	if _, err := runCmd("id", "-u", p.UserName); err == nil {
		return nil // exists
	}
	if out, err := runCmd("id", "-g", p.GroupName); err != nil {
		_ = sudo("groupadd", "-f", p.GroupName)
		_ = out
	}
	home := filepath.Join("/home", p.UserName)
	return sudo("useradd", "-r", "-g", p.GroupName, "-d", home, "-s", "/usr/sbin/nologin", p.UserName)
}

func (p *AgentProvision) addDockerGroup() error {
	if _, err := runCmd("getent", "group", "docker"); err != nil {
		_ = sudo("groupadd", "docker")
	}
	return sudo("usermod", "-aG", "docker", p.UserName)
}

func (p *AgentProvision) createDirs() error {
	dirs := []string{p.DataDir, filepath.Join(p.DataDir, "apps"), filepath.Join(p.DataDir, ".ssh"), p.EnvDir}
	for _, d := range dirs {
		if err := sudo("mkdir", "-p", d); err != nil {
			return err
		}
	}
	_ = sudo("chown", "-R", p.UserName+":"+p.GroupName, p.DataDir)
	_ = sudo("chmod", "700", filepath.Join(p.DataDir, ".ssh"))
	return nil
}

// udevRuleFile is where the hardware-monitoring udev rules are installed.
const udevRuleFile = "/etc/udev/rules.d/99-horizonx-hwmon.rules"

// udevRulesContent returns the udev rules granting the unprivileged agent
// read access to hardware metrics: Intel RAPL powercap, hwmon sensors,
// thermal zones, and block device stats. Ported verbatim from
// scripts/install-agent.sh.
func udevRulesContent() string {
	return `# Intel RAPL power metrics
SUBSYSTEM=="powercap", KERNEL=="intel-rapl:*", ACTION=="add", RUN+="/bin/chmod 444 /sys/class/powercap/%k/energy_uj"
SUBSYSTEM=="powercap", KERNEL=="intel-rapl:*", ACTION=="add", RUN+="/bin/chmod 444 /sys/class/powercap/%k/max_energy_range_uj"

# Hardware monitoring sensors (temp, fan, voltage, power)
SUBSYSTEM=="hwmon", ACTION=="add", RUN+="/bin/chmod -R a+r /sys/class/hwmon/%k"

# Thermal zones
SUBSYSTEM=="thermal", ACTION=="add", RUN+="/bin/chmod 444 /sys/class/thermal/%k/temp"

# Block devices
SUBSYSTEM=="block", ACTION=="add", RUN+="/bin/chmod 444 /sys/block/%k/stat"
`
}

// writeUdevRules installs the hardware-monitoring udev rules, then reloads
// and re-triggers udev so already-present devices get the new permissions
// without a reboot. The udevadm calls are best-effort (the script runs them
// with `|| true`); only the rules install is fatal. Also applies the
// script's immediate cpufreq chmod (scaling_*_freq is read by the CPU
// collector but has no matching udev subsystem rule).
func (p *AgentProvision) writeUdevRules() error {
	tmp := filepath.Join(os.TempDir(), "99-horizonx-hwmon.rules")
	if err := os.WriteFile(tmp, []byte(udevRulesContent()), 0o644); err != nil {
		return err
	}
	if err := sudo("cp", tmp, udevRuleFile); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	_, _ = runCmd("udevadm", "control", "--reload-rules")
	_, _ = runCmd("udevadm", "trigger", "--subsystem-match=powercap")
	_, _ = runCmd("udevadm", "trigger", "--subsystem-match=hwmon")
	_, _ = runCmd("udevadm", "trigger", "--subsystem-match=thermal")
	_, _ = runCmd("udevadm", "trigger", "--subsystem-match=block")
	_, _ = runCmd("chmod", "444", "/sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq")
	return nil
}

func (p *AgentProvision) genSSHKey() error {
	key := filepath.Join(p.DataDir, ".ssh", "id_ed25519")
	if _, err := sudoOut("test", "-f", key); err == nil {
		return nil // exists
	}
	cmd := exec.Command("sudo", "-u", p.UserName, "env",
		"HOME="+p.DataDir,
		"ssh-keygen", "-t", "ed25519", "-f", key, "-N", "", "-C", "horizonx-agent@"+hostname())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (p *AgentProvision) writeSSHConfig() error {
	sshDir := filepath.Join(p.DataDir, ".ssh")
	key := filepath.Join(sshDir, "id_ed25519")
	cfg := fmt.Sprintf(`Host *
  IdentityFile %s
  UserKnownHostsFile %s/known_hosts
  StrictHostKeyChecking yes
  IdentitiesOnly yes
`, key, sshDir)
	dest := filepath.Join(sshDir, "config")
	return p.writeSSHConfigTo(dest, []byte(cfg))
}

// writeSSHConfigTo writes the ssh config to dest via a real temp file. The
// temp file must NOT be the destination: writing the destination directly
// then `sudo cp X X` errors "same file", and the cleanup remove deletes the
// config we just wrote (the exact failure on creatokuserver: chown: cannot
// access '/var/lib/horizonx/.ssh/config'). Separated so tests can fake the
// privileged copy without needing real sudo.
func (p *AgentProvision) writeSSHConfigTo(dest string, cfg []byte) error {
	tmp := filepath.Join(os.TempDir(), "horizonx-ssh-config")
	if err := os.WriteFile(tmp, cfg, 0o644); err != nil {
		return err
	}
	if err := sudo("cp", tmp, dest); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	return sudo("chown", p.UserName+":"+p.GroupName, dest)
}

// gitProviders are the common git hosting hosts we pre-seed into known_hosts
// so the agent's git deploys don't fail host-key verification on first pull.
// Ported from scripts/install-agent.sh.
var gitProviders = []string{
	"github.com",
	"gitlab.com",
	"bitbucket.org",
	"ssh.dev.azure.com",
	"vs-ssh.visualstudio.com",
}

// populateKnownHosts seeds the agent's known_hosts with the hashed public
// keys of the common git providers (ssh-keyscan -H -t rsa,ed25519), skipping
// any already present, then sort -u. The SSH config enforces
// StrictHostKeyChecking yes + UserKnownHostsFile …/known_hosts, so without
// this step the first git deploy would hang/fail on unknown host. Ported
// verbatim from scripts/install-agent.sh (the "Auto-add Git Provider Known
// Hosts" block). Best-effort: a provider that can't be scanned is skipped
// rather than failing the whole install.
func (p *AgentProvision) populateKnownHosts() error {
	sshDir := filepath.Join(p.DataDir, ".ssh")
	knownHosts := filepath.Join(sshDir, "known_hosts")

	// Read existing entries (if any) so we don't duplicate.
	existing := map[string]bool{}
	if data, err := sudoOut("cat", knownHosts); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Hashed entries look like "|1|...|... hostkey"; match on the
			// trailing hostkey token the agent would look up. Simplest
			// robust check: skip a provider if its hashed marker already
			// lines the file. We key on the whole line after de-dup below.
			existing[line] = true
		}
	}

	var additions []string
	for _, provider := range gitProviders {
		// Already covered by an existing line? Cheap prefix check on the
		// hashed format "|1|…|<base64 host> …" — base64 host encodes the
		// provider, so just skip if any existing line already references it.
		if sudoOutHasHost(knownHosts, provider) {
			continue
		}
		out, err := runCmd("ssh-keyscan", "-H", "-t", "rsa,ed25519", provider)
		if err != nil || strings.TrimSpace(out) == "" {
			// Best-effort: skip un-scannable providers.
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !existing[line] {
				additions = append(additions, line)
				existing[line] = true
			}
		}
	}

	if len(additions) == 0 {
		// Nothing new to add (all already present, or none scannable).
		// Ensure the file at least exists with correct perms.
		_ = sudo("touch", knownHosts)
		_ = sudo("chown", p.UserName+":"+p.GroupName, knownHosts)
		_ = sudo("chmod", "644", knownHosts)
		return nil
	}

	// Append additions, then sort -u in place for clean de-dup.
	tmp := filepath.Join(os.TempDir(), "horizonx-known-hosts")
	if data, err := sudoOut("cat", knownHosts); err == nil {
		if err := os.WriteFile(tmp, []byte(strings.TrimSpace(data)+"\n"), 0o644); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(tmp, nil, 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(tmp, []byte(strings.Join(additions, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	if err := sudo("cp", tmp, knownHosts); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	_ = sudo("chown", p.UserName+":"+p.GroupName, knownHosts)
	_ = sudo("chmod", "644", knownHosts)
	// Best-effort sort -u (not all hosts have sort; failure is non-fatal).
	_, _ = runCmd("sh", "-c", fmt.Sprintf("sort -u %s -o %s", knownHosts, knownHosts))
	return nil
}

// sudoOutHasHost reports whether the (hashed) known_hosts file already
// contains an entry for provider. ssh-keyscan -H hashes the hostname, so we
// can't string-match the name directly; instead we re-scan the provider and
// compare the resulting hashed lines against what's already in the file. If
// our fresh scan yields at least one line already present, the host is
// covered.
func sudoOutHasHost(knownHosts, provider string) bool {
	out, err := runCmd("ssh-keyscan", "-H", "-t", "rsa,ed25519", provider)
	if err != nil {
		return false
	}
	cur, err := sudoOut("cat", knownHosts)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(cur, line) {
			return true
		}
	}
	return false
}

func (p *AgentProvision) writeEnvFile() error {
	env := fmt.Sprintf(`HORIZONX_API_URL=%s
HORIZONX_WS_URL=%s
HORIZONX_SERVER_ID=%s
HORIZONX_SERVER_API_TOKEN=%s
REDIS_ADDR=%s
`, p.APIURL, p.WSURL, p.ServerID, p.ServerToken, p.RedisAddr)
	tmp := filepath.Join(os.TempDir(), "horizonx-agent.env")
	if err := os.WriteFile(tmp, []byte(env), 0o600); err != nil {
		return err
	}
	dest := filepath.Join(p.EnvDir, "agent.env")
	if err := sudo("cp", tmp, dest); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	_ = sudo("chown", p.UserName+":"+p.GroupName, dest)
	return sudo("chmod", "600", dest)
}

func (p *AgentProvision) installSystemdUnit() error {
	data, err := systemdUnit("horizonx-agent.service")
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), "horizonx-agent.service")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	dest := filepath.Join("/etc/systemd/system", "horizonx-agent.service")
	if err := sudo("cp", tmp, dest); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	return nil
}

func (p *AgentProvision) enableService() error {
	_ = sudo("systemctl", "daemon-reload")
	return sudo("systemctl", "enable", "--now", "horizonx-agent")
}

// PublicKey prints the agent's public SSH key (for adding to GitHub).
func (p *AgentProvision) PublicKey() (string, error) {
	key := filepath.Join(p.DataDir, ".ssh", "id_ed25519.pub")
	data, err := os.ReadFile(key)
	if err != nil {
		return "", fmt.Errorf("read public key: %w (run: sudo cat %s)", err, key)
	}
	return strings.TrimSpace(string(data)), nil
}

// --- helpers ---------------------------------------------------------------

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(bytes.TrimSpace(out)), err
}

// sudo runs a privileged command. Package var so tests can fake it without
// requiring real passwordless sudo (same pattern as runCommand/execCompose).
var sudo = func(args ...string) error {
	cmd := exec.Command("sudo", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sudo %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func sudoOut(args ...string) (string, error) {
	cmd := exec.Command("sudo", args...)
	out, err := cmd.Output()
	return string(bytes.TrimSpace(out)), err
}

func hostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "localhost"
}
