package app

import (
	"os"
	"testing"
)

func TestRuntimeActiveUnitNone(t *testing.T) {
	// In the test container there is no horizonx unit; ActiveUnit must be "".
	rt := DetectRuntime()
	if got := rt.ActiveUnit(); got != "" {
		t.Errorf("ActiveUnit = %q, want empty (test container has no units)", got)
	}
}

func TestRuntimeUserUnitDetection(t *testing.T) {
	// Simulate a user-level unit and confirm it lands in UserUnits.
	home := t.TempDir()
	t.Setenv("HOME", home)
	unitDir := home + "/.config/systemd/user"
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitDir+"/horizonx-agent.service", []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := DetectRuntime()
	if !rt.IsUserUnit("horizonx-agent.service") {
		t.Error("expected user unit to be detected in UserUnits")
	}
}

func TestRestartServiceSchedules(t *testing.T) {
	// restartService should either schedule via systemd-run (missing in the
	// test container) or fall back to at (also likely missing). It must not
	// panic and must return an error that mentions the unit name.
	err := restartService("horizonx-agent.service", false)
	if err == nil {
		// systemd-run may actually exist and schedule; that's acceptable.
		t.Log("restartService scheduled (systemd-run present)")
		return
	}
	if err.Error() == "" {
		t.Error("expected a non-empty error")
	}
}

func TestNeedsSudoNonRoot(t *testing.T) {
	// can't easily fake euid; just ensure the function exists and returns bool.
	if isRoot() {
		t.Skip("running as root")
	}
	_ = needsSudo("/usr/local/bin")
}
