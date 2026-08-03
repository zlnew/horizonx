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
	tmp := filepath.Join(sshDir, "config")
	if err := os.WriteFile(tmp, []byte(cfg), 0o644); err != nil {
		return err
	}
	_ = sudo("cp", tmp, filepath.Join(sshDir, "config"))
	_ = os.Remove(tmp)
	return sudo("chown", p.UserName+":"+p.GroupName, filepath.Join(sshDir, "config"))
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

func sudo(args ...string) error {
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
