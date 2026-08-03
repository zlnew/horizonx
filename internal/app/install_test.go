package app

import (
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
