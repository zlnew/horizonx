package app

// `horizonx install server` — install OR upgrade the HorizonX docker bubble.
//
// Flow (locked with Maul, 2026-08-03):
//   1. preflight   — probe docker socket access, compose >= 2.20, ports free
//   2. generate    — write the full bubble tree at /opt/horizonx
//   3. validate    — docker compose config --quiet (fail fast, readable error)
//   4. apply       — docker compose up -d (stream output; re-run command on error)
//   5. verify      — poll GET /health until the control plane answers
//
// --generate-only stops after step 2 (no privileged steps, no docker calls).
// Dashboard is BUNDLED (MVP): the dashboard sub-project is part of the same
// bubble, so there is no separate `install dashboard` command.

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// bubbleDir is where the docker bubble lives (root compose + sub-projects).
const bubbleDir = "/opt/horizonx"

// execCompose is the indirection point for docker compose invocations in the
// install-server apply path, so tests can fake the daemon. Defaults to real
// exec; tests override it. The variadic includes the compose subcommand
// (e.g. "config", "up", "-d").
var execCompose = func(args ...string) (string, error) {
	return runCommand(context.Background(), "docker", append([]string{"compose"}, args...)...)
}

// InstallServerOptions carries the flags for `horizonx install server`.
type InstallServerOptions struct {
	Dir          string // bubble dir (default /opt/horizonx)
	Host         string // public host agents/dashboard use
	Admin        string // admin email (accepted, printed; provisioning of the first user is post-boot)
	GenerateOnly bool
	Yes          bool // non-interactive
}

// RunInstallServer installs or upgrades the HorizonX docker bubble.
func RunInstallServer(opts InstallServerOptions) error {
	dir := opts.Dir
	if dir == "" {
		dir = bubbleDir
	}
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}

	fmt.Println("horizonx install server — HorizonX docker bubble")
	fmt.Println()

	// 1. Preflight — probe capabilities, not binaries.
	r := preflightFn()
	if !r.DockerAccess {
		if r.DockerGroupHint != "" {
			return fmt.Errorf("docker is installed but the socket is not accessible.\n  Fix: %s\n  Re-run `horizonx install server` afterwards.", r.DockerGroupHint)
		}
		return fmt.Errorf("docker not found in PATH.\n  Install docker first: https://docs.docker.com/engine/install/\n  Then re-run `horizonx install server`.")
	}
	if !r.ComposeOK {
		return fmt.Errorf("docker compose is too old (%s).\n  HorizonX needs Compose v2.20+ (the `include:` feature).\n  Update docker compose, then re-run.", r.ComposeVersion)
	}
	if !r.PortsFree {
		return fmt.Errorf("signature ports are already in use (server %s / dashboard %s).\n  Free them or set HORIZONX_PORT / DASHBOARD_PORT in the .env.", ServerPort, DashboardPort)
	}
	fmt.Printf("  preflight: docker OK · compose %s OK · ports %s/%s free\n", r.ComposeVersion, ServerPort, DashboardPort)

	// 2. Generate the bubble tree.
	l, err := GenerateBubble(dir, host)
	if err != nil {
		return fmt.Errorf("generate bubble: %w", err)
	}
	fmt.Printf("  generated: %s (root compose + server/ + dashboard/)\n", l.Root)

	if opts.GenerateOnly {
		fmt.Println()
		fmt.Println("✔ generate-only — nothing applied. Next steps:")
		fmt.Printf("  1. cd %s\n", l.Root)
		fmt.Println("  2. docker compose up -d          # postgres + redis + server + dashboard")
		fmt.Printf("  3. open http://<host>:%s        # dashboard\n", DashboardPort)
		return nil
	}

	// 3. Validate the compose before applying.
	fmt.Println("  validating compose…")
	if out, err := execCompose("-f", filepath.Join(l.Root, "docker-compose.yml"), "config", "--quiet"); err != nil {
		return fmt.Errorf("compose config invalid:\n%s\n  Re-run: cd %s && docker compose config", strings.TrimSpace(out), l.Root)
	}

	// 4. Apply. The CORE bubble (postgres + redis + server) comes up first;
	//    the dashboard is best-effort because its image is loaded locally from
	//    a release tarball and may not be present yet. Starting the core
	//    separately means a missing dashboard image can never take down the
	//    control plane.
	fmt.Println("  starting core bubble (postgres + redis + server)…")
	out, err := execCompose("-f", filepath.Join(l.Root, "docker-compose.yml"), "up", "-d", "postgres", "redis", "server")
	if err != nil {
		return fmt.Errorf("docker compose up failed: %s\n  Re-run: cd %s && docker compose up -d\n  (raw error above)", strings.TrimSpace(out), l.Root)
	}

	// 4b. Dashboard — load the image if a tarball is available, then start it.
	if tarball := findDashboardTarball(l.DashboardDir); tarball != "" {
		fmt.Printf("  loading dashboard image (%s)…\n", tarball)
		if err := loadDockerImage(tarball); err != nil {
			fmt.Printf("  ⚠ dashboard image load failed: %v (dashboard will be skipped)\n", err)
		} else if upOut, upErr := execCompose("-f", filepath.Join(l.Root, "docker-compose.yml"), "up", "-d", "dashboard"); upErr != nil {
			fmt.Printf("  ⚠ dashboard start failed: %s (run later: docker compose -f %s up -d dashboard)\n", strings.TrimSpace(upOut), filepath.Join(l.Root, "docker-compose.yml"))
		} else {
			fmt.Println("  dashboard started")
		}
	} else {
		fmt.Printf("  ⚠ no dashboard image tarball found in %s — dashboard not started.\n", l.DashboardDir)
		fmt.Println("    Load the dashboard release tarball into the dir and re-run: docker compose -f " +
			filepath.Join(l.Root, "docker-compose.yml") + " up -d dashboard")
	}

	// 5. Verify — poll the control plane health endpoint.
	fmt.Printf("  waiting for control plane on http://127.0.0.1:%s/health…\n", ServerPort)
	ok := pollHealthFn("http://127.0.0.1:" + ServerPort + "/health")
	if !ok {
		return fmt.Errorf("control plane did not become healthy within 30s.\n  Check: docker compose -f %s logs server\n  Re-run: cd %s && docker compose up -d", filepath.Join(l.Root, "docker-compose.yml"), l.Root)
	}

	fmt.Println()
	fmt.Println("✔ HorizonX bubble is live.")
	fmt.Printf("  Control plane : http://%s:%s\n", host, ServerPort)
	fmt.Printf("  Dashboard     : http://%s:%s\n", host, DashboardPort)
	if opts.Admin != "" {
		fmt.Printf("  First login   : %s (password set during first boot)\n", opts.Admin)
	} else {
		fmt.Printf("  First login   : admin@horizonx.local (password set during first boot)\n")
	}
	fmt.Println()
	fmt.Println("Install the agent on app hosts:")
	fmt.Printf("  curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | HORIZONX_SERVER_ID=… HORIZONX_SERVER_API_TOKEN=… HORIZONX_API_URL=http://%s:%s HORIZONX_WS_URL=ws://%s:%s/ws/agent bash\n", host, ServerPort, host, ServerPort)
	fmt.Println("  (or run `horizonx install agent` on the same box — it reads credentials from the server .env)")
	return nil
}

// preflightFn is the indirection point for the capability probe, so tests can
// fake a box without docker. Defaults to the real probe.
var preflightFn = RunPreflight

// pollHealthFn is the indirection point for the post-apply health check, so
// tests can fake a healthy control plane without binding the signature port.
var pollHealthFn = func(url string) bool {
	return pollHealth(url, 30*time.Second)
}

// pollHealth polls url until it returns 200 or the timeout elapses.
func pollHealth(url string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(2 * time.Second)
	}
}

// InstallServerFlags parses `horizonx install server` flags.
func InstallServerFlags(args []string) (InstallServerOptions, error) {
	fs := flag.NewFlagSet("install server", flag.ExitOnError)
	var opts InstallServerOptions
	fs.StringVar(&opts.Dir, "dir", "", "bubble directory (default /opt/horizonx)")
	fs.StringVar(&opts.Host, "host", "", "public host/IP agents + dashboard use (default 127.0.0.1)")
	fs.StringVar(&opts.Admin, "admin", "", "admin email for the first dashboard user")
	fs.BoolVar(&opts.GenerateOnly, "generate-only", false, "write files only, skip apply")
	fs.BoolVar(&opts.Yes, "yes", false, "non-interactive")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}

// findDashboardTarball looks for a dashboard image tarball in dir. Naming
// convention: horizonx-dashboard-*.tar.gz / *.tgz (the `docker save` artifact
// shipped with the dashboard release).
func findDashboardTarball(dir string) string {
	for _, pattern := range []string{"horizonx-dashboard-*.tar.gz", "horizonx-dashboard-*.tgz"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err == nil && len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

// loadDockerImage runs `docker load -i tarball` (best-effort check via the
// compose executor path would double-wrap; use docker directly).
func loadDockerImage(tarball string) error {
	_, err := runCommand(context.Background(), "docker", "load", "-i", tarball)
	return err
}
