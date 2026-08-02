package app

import (
	"os"
	"strings"
	"testing"
)

func TestModeIndex(t *testing.T) {
	cases := map[WizardMode]int{
		ModeFull:      0,
		ModeServer:    1,
		ModeAgent:     2,
		ModeDashboard: 3,
	}
	for mode, want := range cases {
		if got := modeIndex(mode); got != want {
			t.Errorf("modeIndex(%s) = %d, want %d", mode, got, want)
		}
	}
}

func TestPromptAskNoTTY(t *testing.T) {
	p := &Prompt{NoTTY: true}
	if got := p.Ask("Host?", "default"); got != "default" {
		t.Errorf("Ask in NoTTY mode = %q, want default", got)
	}
}

func TestPromptChooseNoTTY(t *testing.T) {
	p := &Prompt{NoTTY: true}
	opts := []string{"a", "b", "c"}
	if got := p.Choose("Pick", opts, 1); got != 1 {
		t.Errorf("Choose in NoTTY mode = %d, want 1", got)
	}
}

func TestPromptChooseInteractive(t *testing.T) {
	p := &Prompt{In: strings.NewReader("2\n"), NoTTY: false}
	opts := []string{"a", "b", "c"}
	if got := p.Choose("Pick", opts, 0); got != 1 {
		t.Errorf("Choose interactive = %d, want 1", got)
	}
}

func TestPromptAskInteractive(t *testing.T) {
	p := &Prompt{In: strings.NewReader("203.0.113.10\n"), NoTTY: false}
	if got := p.Ask("Host?", "127.0.0.1"); got != "203.0.113.10" {
		t.Errorf("Ask interactive = %q, want typed value", got)
	}
}

func TestPromptConfirmInteractive(t *testing.T) {
	p := &Prompt{In: strings.NewReader("n\n"), NoTTY: false}
	if got := p.Confirm("Continue?", true); got {
		t.Error("Confirm with 'n' should return false")
	}
}

func TestFindServerCredentialsFromEnv(t *testing.T) {
	t.Setenv("HORIZONX_SERVER_ID", "env-id")
	t.Setenv("HORIZONX_SERVER_API_TOKEN", "env-token")
	id, secret := findServerCredentials()
	if id != "env-id" || secret != "env-token" {
		t.Errorf("expected env creds, got %q / %q", id, secret)
	}
}

func TestFindServerCredentialsFromEnvFile(t *testing.T) {
	t.Setenv("HORIZONX_SERVER_ID", "")
	t.Setenv("HORIZONX_SERVER_API_TOKEN", "")
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("HORIZONX_SERVER_ID=file-id\nHORIZONX_SERVER_API_TOKEN=file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// findServerCredentials checks relative paths; simulate by chdir.
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	id, secret := findServerCredentials()
	if id != "file-id" || secret != "file-token" {
		t.Errorf("expected file creds, got %q / %q", id, secret)
	}
}

func TestFindServerCredentialsEmpty(t *testing.T) {
	t.Setenv("HORIZONX_SERVER_ID", "")
	t.Setenv("HORIZONX_SERVER_API_TOKEN", "")
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	_ = os.Chdir(t.TempDir())
	id, secret := findServerCredentials()
	if id != "" || secret != "" {
		t.Errorf("expected empty creds, got %q / %q", id, secret)
	}
}

func TestNeedsSudo(t *testing.T) {
	// Running as root in the test container: never needs sudo.
	if needsSudo("/usr/local/bin") {
		t.Error("root should never need sudo")
	}
}

func TestWizardServerNonInteractive(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	_ = os.Chdir(dir)

	err := RunSetupWizard(wizardOptions{
		mode:         ModeServer,
		noTTY:        true,
		dir:          "hx-setup",
		host:         "203.0.113.10",
		generateOnly: true,
	})
	if err != nil {
		t.Fatalf("RunSetupWizard: %v", err)
	}
	for _, f := range []string{"hx-setup/.env", "hx-setup/docker-compose.yml", "hx-setup/systemd/horizonx-server.service"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected %s to be written: %v", f, err)
		}
	}
}

func TestWizardAgentWithoutCredentials(t *testing.T) {
	t.Setenv("HORIZONX_SERVER_ID", "")
	t.Setenv("HORIZONX_SERVER_API_TOKEN", "")
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	_ = os.Chdir(t.TempDir())

	err := RunSetupWizard(wizardOptions{mode: ModeAgent, noTTY: true})
	if err == nil {
		t.Fatal("expected error when agent mode has no server credentials")
	}
}
