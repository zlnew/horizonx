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
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
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
	Admin        string // admin email (prompted if empty; default admin@horizonx.local)
	AdminPass    string // admin password (prompted if empty; random if blank)
	GenerateOnly bool
	Yes          bool // non-interactive (no prompts; default email + random password)
	ResetVolumes bool // wipe existing bubble volumes first (data loss!)
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

	// First install = .env absent. Prompt for the admin credentials once so
	// they can be printed after apply (re-runs keep the existing .env —
	// install-or-upgrade idempotency — so no prompt).
	firstInstall := false
	if _, err := os.Stat(filepath.Join(dir, ".env")); os.IsNotExist(err) {
		firstInstall = true
	}
	adminEmail := opts.Admin
	adminPass := opts.AdminPass
	if firstInstall && !opts.Yes {
		adminEmail = promptString("Admin email", firstNonEmpty(adminEmail, "admin@horizonx.local"))
		adminPass = promptString("Admin password (blank = random)", adminPass)
	}
	if adminEmail == "" {
		adminEmail = "admin@horizonx.local"
	}

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
	if !r.PortsFree && firstInstall {
		return fmt.Errorf("signature ports are already in use (server %s / dashboard %s).\n  Free them or set HORIZONX_PORT / DASHBOARD_PORT in the .env.", ServerPort, DashboardPort)
	}
	if firstInstall {
		fmt.Printf("  preflight: docker OK · compose %s OK · ports %s/%s free\n", r.ComposeVersion, ServerPort, DashboardPort)
	} else {
		fmt.Printf("  preflight: docker OK · compose %s OK · upgrade path (ports owned by existing bubble)\n", r.ComposeVersion)
	}

	// 2. Generate the bubble tree.
	l, err := GenerateBubbleWithAdmin(dir, host, adminEmail, adminPass)
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

	// 2b. Stale-volume guard: postgres only sets its password on FIRST volume
	// init. If the volume already exists (from an earlier install) but the
	// .env was regenerated, the passwords won't match and the server crash-
	// loops with `password authentication failed for user "postgres"`. Block
	// with the exact remediation instead of letting it fail 30s later.
	if firstInstall && !opts.ResetVolumes {
		if hasBubbleVolumes(l.Root) {
			return fmt.Errorf(`stale bubble volumes detected: a postgres/redis volume from a previous
install exists, but no .env was found — the volume's password differs from the
freshly generated one, so the server would crash-loop with "password
authentication failed for user postgres".

Fix (wipes postgres+redis DATA — safe if you don't need the old data):
  docker compose -f %s down -v
  sudo horizonx install server

Or restore the previous .env into %s and re-run.`, filepath.Join(l.Root, "docker-compose.yml"), l.Root)
		}
	}
	if opts.ResetVolumes {
		fmt.Println("  resetting bubble volumes (--reset-volumes: data wiped)…")
		if out, err := execCompose("-f", filepath.Join(l.Root, "docker-compose.yml"), "down", "-v"); err != nil {
			return fmt.Errorf("reset volumes failed: %s", strings.TrimSpace(out))
		}
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
	// On the upgrade path the server container may already exist and be
	// running — a plain `up -d` does NOT recreate it, so the process never
	// restarts and the boot-time admin auto-seed (AUTO_SEED) never re-runs
	// (the exact "admin missing after DELETE + re-run install" bug Maul hit).
	// env_file is read at container create, so a changed .env also never
	// reaches a running container without a recreate. Force-recreate ONLY the
	// stateless server container — postgres/redis keep their volumes and data.
	if !firstInstall {
		fmt.Println("  refreshing server container (--force-recreate)…")
		if out2, err2 := execCompose("-f", filepath.Join(l.Root, "docker-compose.yml"), "up", "-d", "--force-recreate", "--no-deps", "server"); err2 != nil {
			return fmt.Errorf("docker compose up --force-recreate server failed: %s\n  (raw error above)", strings.TrimSpace(out2))
		}
	}

	// 4b. Dashboard — fetch the latest dashboard release automatically, load
	//     the image, then start it. Best-effort: any dashboard failure (network
	//     down, release API unreachable, checksum mismatch) warns and skips —
	//     a missing dashboard can never take down the control plane.
	if tarball, err := fetchDashboardTarball(l.DashboardDir); err != nil {
		fmt.Printf("  ⚠ dashboard image not available: %v\n", err)
		fmt.Println("    The control plane is up; add the dashboard later:")
		fmt.Println("      docker compose -f " + filepath.Join(l.Root, "docker-compose.yml") + " up -d dashboard")
	} else {
		fmt.Printf("  loading dashboard image (%s)…\n", filepath.Base(tarball))
		if err := loadDockerImage(tarball); err != nil {
			fmt.Printf("  ⚠ dashboard image load failed: %v (dashboard will be skipped)\n", err)
		} else if upOut, upErr := execCompose("-f", filepath.Join(l.Root, "docker-compose.yml"), "up", "-d", "dashboard"); upErr != nil {
			fmt.Printf("  ⚠ dashboard start failed: %s (run later: docker compose -f %s up -d dashboard)\n", strings.TrimSpace(upOut), filepath.Join(l.Root, "docker-compose.yml"))
		} else {
			fmt.Println("  dashboard started")
		}
	}

	// 5. Verify — poll the control plane health endpoint.
	fmt.Printf("  waiting for control plane on http://127.0.0.1:%s/health…\n", ServerPort)
	ok := pollHealthFn("http://127.0.0.1:" + ServerPort + "/health")
	if !ok {
		// Diagnose the two most common crash-loop causes so the user gets the
		// fix instead of a raw log dump.
		if logs := bubbleServerLogs(l.Root); logs != "" {
			if strings.Contains(logs, "password authentication failed") || strings.Contains(logs, "SQLSTATE 28P01") {
				return fmt.Errorf(`control plane did not become healthy within 30s — the server can't
+authenticate to postgres. The postgres volume was initialized with a password
+from an OLDER .env; the current .env has a different one, so auth fails on
+every boot (postgres only sets its password on first volume init).

+Fix (wipes postgres+redis DATA — safe if you don't need the old data):
+  docker compose -f %s down -v
+  sudo horizonx install server

+Or restore the previous .env into %s and re-run.`, filepath.Join(l.Root, "docker-compose.yml"), l.Root)
			}
			if strings.Contains(logs, "redis") && strings.Contains(logs, "NOAUTH") {
				return fmt.Errorf("control plane did not become healthy within 30s — redis auth mismatch.\n  Check: docker compose -f %s logs server", filepath.Join(l.Root, "docker-compose.yml"))
			}
		}
		return fmt.Errorf("control plane did not become healthy within 30s.\n  Check: docker compose -f %s logs server\n  Re-run: cd %s && docker compose up -d", filepath.Join(l.Root, "docker-compose.yml"), l.Root)
	}

	fmt.Println()
	fmt.Println("✔ HorizonX bubble is live.")
	fmt.Printf("  Control plane : http://%s:%s\n", host, ServerPort)
	fmt.Printf("  Dashboard     : http://%s:%s\n", host, DashboardPort)

	// Print the admin credentials ONLY on first install — the .env password
	// seeds the admin on first boot and is never re-applied, so on re-runs
	// printing the .env value would show creds that may not match the account
	// (e.g. if the password was changed in the dashboard). First install is
	// when they're genuinely fresh, so they're safe to show then.
	if firstInstall {
		if envData, err := os.ReadFile(l.EnvPath); err == nil {
			email := envValue(envData, "ADMIN_EMAIL")
			pass := envValue(envData, "ADMIN_PASSWORD")
			if email != "" {
				fmt.Println()
				fmt.Println("  Admin credentials (save these — shown once):")
				fmt.Printf("    Email    : %s\n", email)
				fmt.Printf("    Password : %s\n", pass)
				fmt.Println("    Login at  http://" + host + ":" + DashboardPort)
			}
		}
	} else {
		fmt.Println()
		fmt.Println("  Admin account: existing user kept (password not reset by re-runs).")
		fmt.Println("    If you forgot the password, reset it in the dashboard account page")
		fmt.Println("    or re-seed it from .env (wipes just the admin row, keeps all data):")
		fmt.Printf("      docker compose -f %s exec postgres psql -U postgres -d horizonx -c \"DELETE FROM users WHERE email='admin@horizonx.local';\"\n", filepath.Join(l.Root, "docker-compose.yml"))
		fmt.Println("      # set a new ADMIN_PASSWORD in .env first, then:")
		fmt.Printf("      docker compose -f %s up -d --force-recreate server\n", filepath.Join(l.Root, "docker-compose.yml"))
	}
	fmt.Println()
	fmt.Println("Install the agent on app hosts:")
	fmt.Printf("  curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | HORIZONX_SERVER_ID=… HORIZONX_SERVER_API_TOKEN=… HORIZONX_API_URL=http://%s:%s HORIZONX_WS_URL=ws://%s:%s/ws/agent bash\n", host, ServerPort, host, ServerPort)
	fmt.Println("  (or run `horizonx install agent` on the same box — it reads credentials from the server .env)")
	return nil
}

// envValue extracts `KEY=value` from .env-style bytes (first match, trimmed).
func envValue(data []byte, key string) string {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+"="))
		}
	}
	return ""
}

// hasBubbleVolumes reports whether a postgres/redis volume from a previous
// bubble install exists in docker (any compose project prefix). Used by the
// stale-volume guard: postgres only sets its password on FIRST volume init, so
// a surviving volume + freshly regenerated .env = guaranteed auth failure.
func hasBubbleVolumes(dir string) bool {
	out, err := runCommand(context.Background(), "docker", "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return false
	}
	for _, name := range strings.Split(out, "\n") {
		name = strings.TrimSpace(name)
		if name == "horizonx_pgdata" || name == "horizonx_redisdata" ||
			strings.HasSuffix(name, "_horizonx_pgdata") || strings.HasSuffix(name, "_horizonx_redisdata") {
			return true
		}
	}
	return false
}

// bubbleServerLogs returns the last ~60 lines of the server container's log
// (best-effort; empty string when the container isn't running or compose
// fails). Used to diagnose why a freshly applied bubble isn't healthy.
func bubbleServerLogs(dir string) string {
	out, err := execCompose("-f", filepath.Join(dir, "docker-compose.yml"), "logs", "--tail", "60", "server")
	if err != nil {
		return strings.TrimSpace(out)
	}
	return strings.TrimSpace(out)
}

// promptString reads a line from stdin, defaulting to def when blank.
func promptString(label, def string) string {
	fmt.Printf("  %s [%s]: ", label, def)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return def
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// firstNonEmpty returns a when non-empty, else b.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
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
	fs.StringVar(&opts.Admin, "admin", "", "admin email for the first dashboard user (default admin@horizonx.local)")
	fs.StringVar(&opts.AdminPass, "admin-password", "", "admin password (default: random, shown after install)")
	fs.BoolVar(&opts.GenerateOnly, "generate-only", false, "write files only, skip apply")
	fs.BoolVar(&opts.Yes, "yes", false, "non-interactive")
	fs.BoolVar(&opts.ResetVolumes, "reset-volumes", false, "wipe existing bubble postgres/redis volumes first (DATA LOSS)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}

// findDashboardTarball looks for an existing dashboard image tarball in dir.
// Naming convention: horizonx-dashboard-*.tar.gz / *.tgz (the `docker save`
// artifact shipped with the dashboard release). Used as a fallback when the
// release API is unreachable.
func findDashboardTarball(dir string) string {
	for _, pattern := range []string{"horizonx-dashboard-*.tar.gz", "horizonx-dashboard-*.tgz"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err == nil && len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

// dashboardReleaseRepo is where the dashboard image tarball + SHA256SUMS live.
const dashboardReleaseRepo = "zlnew/horizonx-dashboard"

// dashboardRelease describes a published dashboard image tarball.
type dashboardRelease struct {
	Tag        string // e.g. v0.3.0
	TarballURL string // browser_download_url for the image tarball
	SHAURL     string // browser_download_url for SHA256SUMS
}

// latestDashboardRelease resolves the latest dashboard release via the GitHub
// API (unauthenticated; the install command only needs one call). It finds the
// image tarball asset and its SHA256SUMS companion. Package var so tests can
// fake the network (same pattern as execCompose / preflightFn).
var latestDashboardRelease = func() (dashboardRelease, error) {
	var rel dashboardRelease
	apiURL := "https://api.github.com/repos/" + dashboardReleaseRepo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return rel, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return rel, fmt.Errorf("release API returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return rel, err
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return rel, err
	}
	rel.Tag = payload.TagName
	for _, a := range payload.Assets {
		switch {
		case strings.HasPrefix(a.Name, "horizonx-dashboard-") && strings.HasSuffix(a.Name, "-image.tar.gz"):
			rel.TarballURL = a.BrowserDownloadURL
		case a.Name == "SHA256SUMS":
			rel.SHAURL = a.BrowserDownloadURL
		}
	}
	if rel.TarballURL == "" {
		return rel, fmt.Errorf("latest dashboard release %q has no image tarball asset", rel.Tag)
	}
	return rel, nil
}

// downloadFile fetches url into path (overwrites). Returns the bytes written.
func downloadFile(url, path string) (int64, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download %s returned %s", url, resp.Status)
	}
	out, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	return io.Copy(out, resp.Body)
}

// verifySHA256SUMS checks tarballPath against a SHA256SUMS text (the
// "hash  filename" lines format produced by sha256sum). It verifies the exact
// basename of the tarball so a mismatched file can never pass.
func verifySHA256SUMS(tarballPath, sumsText string) error {
	base := filepath.Base(tarballPath)
	var want string
	for _, line := range strings.Split(sumsText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == base {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("SHA256SUMS has no entry for %s", base)
	}
	f, err := os.Open(tarballPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", base, got, want)
	}
	return nil
}

// fetchDashboardTarball returns a verified dashboard image tarball in dir,
// downloading it from the latest dashboard release when needed. Resolution:
//  1. If dir already has a tarball AND it matches the release SHA256SUMS,
//     reuse it (idempotent re-run — no re-download).
//  2. Otherwise download the latest release tarball + SHA256SUMS into dir,
//     verify, and return it.
//  3. If the release API is unreachable, fall back to any local tarball
//     (unverified — caller treats dashboard as best-effort).
func fetchDashboardTarball(dir string) (string, error) {
	rel, err := latestDashboardRelease()
	if err != nil {
		if local := findDashboardTarball(dir); local != "" {
			return local, nil
		}
		return "", fmt.Errorf("cannot resolve latest dashboard release: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Asset names: horizonx-dashboard-<tag>-image.tar.gz (tag INCLUDES the v,
	// e.g. horizonx-dashboard-v0.3.0-image.tar.gz).
	name := "horizonx-dashboard-" + rel.Tag + "-image.tar.gz"
	tarballPath := filepath.Join(dir, name)
	sumsPath := filepath.Join(dir, "SHA256SUMS")

	// Reuse an existing tarball only when it passes the release checksum.
	if _, err := os.Stat(tarballPath); err == nil && rel.SHAURL != "" {
		if _, err := downloadFile(rel.SHAURL, sumsPath); err == nil {
			if sums, rerr := os.ReadFile(sumsPath); rerr == nil && verifySHA256SUMS(tarballPath, string(sums)) == nil {
				return tarballPath, nil
			}
		}
	}

	// Download the tarball fresh.
	if _, err := downloadFile(rel.TarballURL, tarballPath); err != nil {
		return "", fmt.Errorf("download dashboard tarball: %v", err)
	}
	if rel.SHAURL != "" {
		if _, err := downloadFile(rel.SHAURL, sumsPath); err != nil {
			return "", fmt.Errorf("download dashboard SHA256SUMS: %v", err)
		}
		sums, err := os.ReadFile(sumsPath)
		if err != nil {
			return "", err
		}
		if err := verifySHA256SUMS(tarballPath, string(sums)); err != nil {
			return "", err
		}
	}
	return tarballPath, nil
}

// loadDockerImage runs `docker load -i tarball` (best-effort check via the
// compose executor path would double-wrap; use docker directly).
func loadDockerImage(tarball string) error {
	_, err := runCommand(context.Background(), "docker", "load", "-i", tarball)
	return err
}
