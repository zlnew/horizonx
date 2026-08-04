package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUdevRulesContent verifies the ported udev rules cover the four hardware
// subsystems the old shell installer (scripts/install-agent.sh) granted the
// unprivileged horizonx agent read access to: Intel RAPL powercap, hwmon
// sensors, thermal zones, and block device stats.
func TestUdevRulesContent(t *testing.T) {
	content := udevRulesContent()
	for _, want := range []string{"powercap", "hwmon", "thermal", "block"} {
		if !strings.Contains(content, want) {
			t.Errorf("udevRulesContent() missing %q in:\n%s", want, content)
		}
	}
}

// TestUdevRuleFileConst pins the destination the rules are written to.
func TestUdevRuleFileConst(t *testing.T) {
	const want = "/etc/udev/rules.d/99-horizonx-hwmon.rules"
	if udevRuleFile != want {
		t.Errorf("udevRuleFile = %q, want %q", udevRuleFile, want)
	}
}

// TestProvisionStepsIncludeUdevRules pins the udev rules step in the agent
// provisioning sequence, right after the directories step.
func TestProvisionStepsIncludeUdevRules(t *testing.T) {
	p := &AgentProvision{}
	var names []string
	for _, s := range p.provisionSteps() {
		names = append(names, s.name)
	}
	found := -1
	for i, n := range names {
		if n == "udev rules" {
			found = i
			break
		}
	}
	if found < 0 {
		t.Fatalf("provision steps %v missing \"udev rules\"", names)
	}
	if found == 0 || names[found-1] != "create directories" {
		t.Errorf("udev rules step at index %d, want it directly after \"create directories\": %v", found, names)
	}
}

// TestWriteSSHConfigTempPath pins the self-destruct fix: the temp file must
// NOT be the destination path (writing the destination then `sudo cp X X`
// errors "same file" and the cleanup remove deletes the config — the exact
// failure Maul hit on creatokuserver: chown: cannot access
// '/var/lib/horizonx/.ssh/config': No such file or directory).
func TestWriteSSHConfigTempPath(t *testing.T) {
	sshDir := t.TempDir()
	p := &AgentProvision{UserName: "horizonx", GroupName: "horizonx"}
	dest := filepath.Join(sshDir, "config")
	cfg := []byte("Host *\n  IdentityFile " + filepath.Join(sshDir, "id_ed25519") + "\n")
	// Fake sudo as plain cp/chown (this test runs as the owner). chown to a
	// non-existent user would fail without root — skip it.
	oldSudo := sudo
	sudo = func(args ...string) error {
		if args[0] == "chown" {
			return nil
		}
		_, err := runCmd(args[0], args[1:]...)
		return err
	}
	defer func() { sudo = oldSudo }()
	if err := p.writeSSHConfigTo(dest, cfg); err != nil {
		t.Fatalf("writeSSHConfigTo: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("config not written to %s: %v", dest, err)
	}
	if !strings.Contains(string(data), "id_ed25519") {
		t.Errorf("config missing IdentityFile:\n%s", string(data))
	}
	// No stray temp file left behind in the .ssh dir.
	entries, _ := os.ReadDir(sshDir)
	for _, e := range entries {
		if e.Name() != "config" {
			t.Errorf("unexpected file in .ssh: %s", e.Name())
		}
	}
}
