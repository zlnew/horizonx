// The interactive setup wizard.
//
// `horizonx setup` with no flags walks the user through:
//
//	1. Mode        — full | server | agent | dashboard
//	2. Preflight   — dependency checks, missing tools reported up front
//	3. Method      — docker compose (recommended when Docker present) | systemd
//	4. Environment — host, admin email, DB/Redis reachability
//	5. Secrets     — JWT, server id, agent token (generated, never asked)
//	6. Execute     — generate files / provision user + SSH key / enable units
//
// Everything can be answered non-interactively with flags, so it still works
// in scripts and CI. The box answers what it can (install method, deps); the
// wizard only asks what a machine cannot tell us.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// WizardMode is the install target selected by the user.
type WizardMode string

const (
	ModeFull      WizardMode = "full"
	ModeServer    WizardMode = "server"
	ModeAgent     WizardMode = "agent"
	ModeDashboard WizardMode = "dashboard"
)

var modeNames = []string{"full (server + dashboard + agent)", "server only", "agent only", "dashboard only"}

// wizardOptions carries flag overrides for non-interactive runs.
type wizardOptions struct {
	mode         WizardMode
	method       string // "docker" | "systemd" | "" (probe)
	host         string
	admin        string
	dir          string
	noTTY        bool
	httpAddr     string
	generateOnly bool // write files + print instructions, skip privileged steps
}

// RunSetupWizard is the interactive entrypoint.
func RunSetupWizard(opts wizardOptions) error {
	p := NewPrompt()
	if opts.noTTY {
		p.NoTTY = true
	}
	rt := DetectRuntime()

	p.Section("HorizonX setup")
	p.Info("This wizard installs HorizonX on this machine.")
	if !isRoot() && !p.NoTTY {
		p.Info("Privileged steps (user creation, systemd, /etc) will prompt for sudo when needed.")
	}

	// --- 1. Mode ------------------------------------------------------------
	p.Section("1 · What are you installing?")
	// Probe: already-installed units hint at the default.
	defMode := 0
	if rt.HasUnit(agentUnit) && !rt.HasUnit(serverUnit) {
		defMode = 2
	} else if rt.HasUnit(serverUnit) && !rt.HasUnit(agentUnit) {
		defMode = 1
	}
	if opts.mode != "" {
		defMode = modeIndex(opts.mode)
	}
	sel := p.Choose("Choose an install mode:", modeNames, defMode)
	mode := WizardMode([]string{"full", "server", "agent", "dashboard"}[sel])

	// --- 2. Preflight -------------------------------------------------------
	p.Section("2 · Preflight checks")
	deps := Preflight(string(mode))
	missing := deps.Missing()
	if len(missing) == 0 {
		p.Info("All required dependencies are present.")
	} else {
		p.Info("Missing dependencies:")
		for _, d := range missing {
			hint := InstallHint(d.Name)
			if hint == "" {
				hint = d.Hint
			}
			p.Info("  ✗ %-24s %s", d.Name, hint)
		}
		if !p.NoTTY && !p.Confirm("Continue anyway?", false) {
			return fmt.Errorf("aborted: missing dependencies")
		}
		if p.NoTTY {
			// --yes: warn and continue — the user opted into non-interactive.
			p.Info("continuing despite missing dependencies (use --yes to skip this warning).")
		}
	}

	// --- 3. Install method --------------------------------------------------
	p.Section("3 · Install method")
	method := rt.RecommendedMethod()
	if opts.method != "" {
		method = opts.method
	} else if mode == ModeAgent {
		// Agents always run as a systemd unit (they must survive reboot and
		// run under the horizonx user). Docker is for the control plane.
		method = "systemd"
	} else {
		p.Info("Detected: docker=%v systemd=%v → recommend %q", rt.DockerCLI, rt.Systemd, method)
		if p.NoTTY {
			p.Info("using %q", method)
		} else if !p.Confirm(fmt.Sprintf("Install via %s?", method), true) {
			alt := "systemd"
			if method == "systemd" {
				alt = "docker"
			}
			method = alt
		}
	}

	// --- 4. Environment -----------------------------------------------------
	p.Section("4 · Environment")
	host := opts.host
	if host == "" {
		host = detectHost()
	}
	if mode != ModeAgent {
		host = p.Ask("Public IP/FQDN agents use to reach this server", host)
	}
	admin := opts.admin
	if admin == "" {
		admin = p.Ask("Admin email (first dashboard user)", "admin@horizonx.local")
	}
	httpAddr := opts.httpAddr
	if httpAddr == "" {
		httpAddr = ":3000"
	}

	// --- 5. Secrets (generated, never prompted) ----------------------------
	p.Section("5 · Generating secrets")
	// Agent mode MUST reuse the server's credentials — a random token here
	// would be rejected by the server. Pull from an existing .env first.
	var serverID string
	var agentSecret string
	if mode == ModeAgent {
		id, secret := findServerCredentials()
		if id == "" || secret == "" {
			return fmt.Errorf("agent install needs the control plane's credentials; run `horizonx setup` on the server first, or set HORIZONX_SERVER_ID + HORIZONX_SERVER_API_TOKEN")
		}
		serverID, agentSecret = id, secret
	} else {
		serverID = uuid.New().String()
		secret, err := randomHex(32)
		if err != nil {
			return err
		}
		agentSecret = secret
	}
	p.Info("secrets ready (agent token reuses the server's credentials in agent mode).")

	// --- 6. Execute ---------------------------------------------------------
	p.Section("6 · Applying")
	apiURL := "http://" + host + ":3000"
	wsURL := "ws://" + host + ":3000/ws/agent"

	switch mode {
	case ModeAgent:
		return runAgentInstall(p, rt, &AgentProvision{
			APIURL:      apiURL,
			WSURL:       wsURL,
			ServerID:    serverID,
			ServerToken: agentSecret,
			RedisAddr:   p.Ask("Redis address (host:port)", "localhost:6379"),
		})

	case ModeServer, ModeFull, ModeDashboard:
		dir := opts.dir
		if dir == "" {
			dir = "horizonx-setup"
		}
		if err := generateSetup(dir, host, httpAddr, "latest", "latest"); err != nil {
			return err
		}
		p.Info("generated setup files in %s", dir)

		if opts.generateOnly {
			p.Info("generate-only: not installing. Next: cd %s && docker compose up -d (or install the systemd units)", dir)
			break
		}

		if method == "docker" {
			if err := runComposeUp(p, dir); err != nil {
				return err
			}
		} else {
			if err := installServerSystemd(p, dir, host, httpAddr); err != nil {
				return err
			}
		}
	}

	// --- Summary ------------------------------------------------------------
	p.Section("Done")
	switch mode {
	case ModeAgent:
		// handled inside runAgentInstall (needs the public key)
	case ModeServer, ModeFull:
		p.Info("Control plane: %s (dashboard: http://%s:8080)", apiURL, host)
		p.Info("Admin login: %s", admin)
		p.Info("Next: install an agent on each app host, then register it in the dashboard.")
	default:
		p.Info("Dashboard setup complete.")
	}
	return nil
}

func modeIndex(m WizardMode) int {
	switch m {
	case ModeServer:
		return 1
	case ModeAgent:
		return 2
	case ModeDashboard:
		return 3
	default:
		return 0
	}
}

// findServerCredentials returns the control plane's HORIZONX_SERVER_ID and
// HORIZONX_SERVER_API_TOKEN from the environment or an existing .env file.
// Agent installs must reuse these — a fresh random token is rejected.
func findServerCredentials() (string, string) {
	id := os.Getenv("HORIZONX_SERVER_ID")
	secret := os.Getenv("HORIZONX_SERVER_API_TOKEN")
	if id != "" && secret != "" {
		return id, secret
	}

	candidates := []string{
		"/etc/horizonx/.env",
		"horizonx-setup/.env",
		".env",
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var eid, esec string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "HORIZONX_SERVER_ID="):
				eid = strings.TrimPrefix(line, "HORIZONX_SERVER_ID=")
			case strings.HasPrefix(line, "HORIZONX_SERVER_API_TOKEN="):
				esec = strings.TrimPrefix(line, "HORIZONX_SERVER_API_TOKEN=")
			}
		}
		if eid != "" && esec != "" {
			return eid, esec
		}
	}
	return "", ""
}

func detectHost() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "127.0.0.1"
}

// runAgentInstall provisions the agent as a systemd service and prints the
// public SSH key for the git provider.
func runAgentInstall(p *Prompt, rt *Runtime, prov *AgentProvision) error {
	if prov.UserName == "" {
		prov.UserName = "horizonx"
	}
	if prov.GroupName == "" {
		prov.GroupName = "horizonx"
	}
	if prov.DataDir == "" {
		prov.DataDir = "/var/lib/horizonx"
	}
	if prov.EnvDir == "" {
		prov.EnvDir = "/etc/horizonx"
	}
	if prov.ServerID == "" || prov.ServerToken == "" {
		return fmt.Errorf("agent install requires server id + token (run setup --mode full/server first, or pass flags)")
	}

	if err := ProvisionAgent(prov); err != nil {
		return err
	}

	p.Info("Agent installed. Add this public SSH key to your git provider so deploys can clone:")
	key, err := prov.PublicKey()
	if err != nil {
		p.Info("(could not read key: %v)", err)
	} else {
		p.Info("  %s", key)
	}
	p.Info("Service: systemctl status horizonx-agent")
	return nil
}

// runComposeUp brings up a generated setup dir via docker compose.
func runComposeUp(p *Prompt, dir string) error {
	compose := filepath.Join(dir, "docker-compose.yml")
	if _, err := os.Stat(compose); err != nil {
		return fmt.Errorf("compose file missing: %w", err)
	}
	if !p.Confirm("Run `docker compose up -d` now?", true) {
		p.Info("skipped — run it yourself: cd %s && docker compose up -d", dir)
		return nil
	}
	out, err := execCommand("docker", "compose", "-f", compose, "up", "-d").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	fmt.Fprintf(p.out(), "%s\n", strings.TrimSpace(string(out)))
	return nil
}

// installServerSystemd copies env + unit to /etc/horizonx and enables the
// horizonx-server service. Uses sudo for the /etc writes.
func installServerSystemd(p *Prompt, dir, host, httpAddr string) error {
	envSrc := filepath.Join(dir, ".env")
	envDst := "/etc/horizonx/.env"
	if err := sudo("mkdir", "-p", "/etc/horizonx"); err != nil {
		return err
	}
	if err := sudo("cp", envSrc, envDst); err != nil {
		return err
	}
	_ = sudo("chmod", "600", envDst)

	data, err := systemdUnit("horizonx-server.service")
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), "horizonx-server.service")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := sudo("cp", tmp, "/etc/systemd/system/horizonx-server.service"); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	_ = sudo("systemctl", "daemon-reload")
	if p.Confirm("Enable and start horizonx-server now?", true) {
		if err := sudo("systemctl", "enable", "--now", "horizonx-server"); err != nil {
			return err
		}
	}
	p.Info("Server env: %s", envDst)
	p.Info("Service: systemctl status horizonx-server")
	return nil
}
