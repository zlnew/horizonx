package app

// `horizonx install agent` — install OR upgrade the HorizonX agent on this host.
//
// The agent NEVER runs in docker (design rule locked with Maul, 2026-08-03):
// its job is to run docker-compose deploys on the host, read host hardware
// (/sys, hwmon, powercap, nvidia-smi), and survive `docker compose down` to
// redeploy the instance. So provisioning creates a dedicated system user with
// docker group membership, a git SSH key, hwmon udev rules, an env file, and
// a systemd unit — no container involved.
//
// Credentials: the agent authenticates with the token the server generated
// when the server was REGISTERED IN THE DASHBOARD (the servers table stores
// the bcrypt hash; the instance .env's HORIZONX_SERVER_ID/API_TOKEN are
// placeholders that never authenticate). So --token is REQUIRED: pass the
// token shown by the dashboard at registration. --server defaults to the
// instance's control plane URL when run on the same box.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallAgentOptions carries the flags for `horizonx install agent`.
type InstallAgentOptions struct {
	Server string // control-plane base URL (http://host:4858); default: read from instance .env
	Token  string // agent token from dashboard server registration (REQUIRED)
	Yes    bool
}

// RunInstallAgent provisions (or re-provisions, i.e. upgrades) the agent.
func RunInstallAgent(opts InstallAgentOptions) error {
	prov := defaultAgentProvision()

	if opts.Token == "" {
		return fmt.Errorf("agent token is required: register the server in the dashboard, then pass --token <token>")
	}
	if opts.Server == "" {
		if err := loadAgentServerURLFromInstance(prov); err != nil {
			return err
		}
	} else {
		prov.APIURL = strings.TrimRight(opts.Server, "/")
		prov.WSURL = strings.Replace(prov.APIURL, "http://", "ws://", 1) + "/ws/agent"
	}
	// Token format from dashboard registration: "<server-uuid>.<secret>"
	// (ValidateAgentCredentials splits on the first dot). ServerID is the
	// part before the dot; the secret is what the server bcrypt-verifies
	// against the servers table row.
	if i := strings.Index(opts.Token, "."); i > 0 {
		prov.ServerID = opts.Token[:i]
		prov.ServerToken = opts.Token[i+1:]
	} else {
		return fmt.Errorf("invalid agent token: expected <server-id>.<secret> from dashboard registration")
	}

	fmt.Println("horizonx install agent — host systemd agent")
	fmt.Println()
	fmt.Printf("  server    : %s\n", prov.APIURL)
	fmt.Printf("  user      : %s\n", prov.UserName)
	fmt.Printf("  data dir  : %s\n", prov.DataDir)
	fmt.Println()

	if err := ProvisionAgent(prov); err != nil {
		return fmt.Errorf("provision agent: %w", err)
	}

	fmt.Println()
	fmt.Println("✔ Agent provisioned and started.")
	fmt.Println("  SSH key (add to GitHub for git deploys):")
	key := filepath.Join(prov.DataDir, ".ssh", "id_ed25519.pub")
	if data, err := os.ReadFile(key); err == nil {
		fmt.Println("    " + strings.TrimSpace(string(data)))
	} else {
		fmt.Printf("    (see %s)\n", key)
	}
	return nil
}

// loadAgentServerURLFromInstance reads the control-plane URL from the server
// instance's .env at /opt/horizonx (same-box install). The token is NOT read
// from the instance — it must come from dashboard registration (the servers
// table is the source of truth; the .env's HORIZONX_SERVER_ID/API_TOKEN are
// placeholders that never authenticate).
func loadAgentServerURLFromInstance(prov *AgentProvision) error {
	envPath := filepath.Join(instanceDir, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("no --server given and no server instance found at %s.\n"+
			"  On the same box as the server, install the server first (horizonx install server).\n"+
			"  On a different host, pass: horizonx install agent --server http://host:4858 --token <token>", envPath)
	}
	vars := parseEnv(string(data))
	prov.APIURL = vars["HORIZONX_API_URL"]
	prov.WSURL = vars["HORIZONX_WS_URL"]
	if prov.APIURL == "" {
		return fmt.Errorf("server instance .env at %s is missing HORIZONX_API_URL.\n"+
			"  Re-run: horizonx install server --generate-only", envPath)
	}
	return nil
}

// parseEnv parses KEY=VALUE lines (ignores comments/blank). Used to read the
// instance .env for agent credentials.
func parseEnv(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			out[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
		}
	}
	return out
}

// InstallAgentFlags parses `horizonx install agent` flags.
func InstallAgentFlags(args []string) (InstallAgentOptions, error) {
	fs := flag.NewFlagSet("install agent", flag.ExitOnError)
	var opts InstallAgentOptions
	fs.StringVar(&opts.Server, "server", "", "control-plane base URL (http://host:4858); default: read from instance .env")
	fs.StringVar(&opts.Token, "token", "", "agent token from dashboard server registration (REQUIRED: <server-id>.<secret>)")
	fs.BoolVar(&opts.Yes, "yes", false, "non-interactive")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}
