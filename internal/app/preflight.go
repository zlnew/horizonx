// Preflight dependency checks for the setup wizard.
//
// The wizard should never ask a question it can answer by probing the box.
// Preflight checks for the tools HorizonX needs and reports exactly what's
// missing, with the command to install it — instead of failing 3 steps later.
package app

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Dependency is one preflight check result.
type Dependency struct {
	Name    string // e.g. "git"
	Present bool
	Hint    string // install command / explanation when missing
}

// Preflight runs the dependency checks relevant to the requested mode.
// mode is one of "full", "server", "agent", "dashboard".
func Preflight(mode string) Deps {
	var deps []Dependency

	// Universal tools.
	deps = append(deps, checkBin("git", "git is needed to clone app repositories"))
	deps = append(deps, checkBin("curl", "curl is needed by the one-line installer"))
	deps = append(deps, checkBin("tar", "tar is needed to unpack release tarballs"))

	// Server + full need a database and redis.
	if mode == "server" || mode == "full" {
		deps = append(deps, Dependency{
			Name:    "postgres (reachable)",
			Present: tcpReachable("localhost", "5432"),
			Hint:    "start postgres, or run the compose stack: docker compose up -d postgres",
		})
		deps = append(deps, Dependency{
			Name:    "redis (reachable)",
			Present: tcpReachable("localhost", "6379"),
			Hint:    "start redis, or run the compose stack: docker compose up -d redis",
		})
	}

	// Agent needs docker + compose (it deploys via docker compose).
	if mode == "agent" || mode == "full" {
		deps = append(deps, checkBin("docker", "the agent deploys apps with docker compose"))
		deps = append(deps, checkCompose("docker compose", "the agent deploys apps with docker compose"))
	}

	return deps
}

func checkBin(name, hint string) Dependency {
	_, err := exec.LookPath(name)
	return Dependency{Name: name, Present: err == nil, Hint: hint}
}

func checkCompose(name, hint string) Dependency {
	out, err := exec.Command("docker", "compose", "version").CombinedOutput()
	// Accept any docker compose v2+ output ("Docker Compose version v2.x" / "5.x").
	present := err == nil && strings.Contains(strings.ToLower(string(out)), "compose version")
	return Dependency{Name: name, Present: present, Hint: hint}
}

// tcpReachable tries a short TCP dial; used for postgres/redis probes.
func tcpReachable(host, port string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Deps is a named slice so we can attach helpers.
type Deps []Dependency

// Missing returns the deps that failed, for reporting.
func (deps Deps) Missing() Deps {
	var m Deps
	for _, d := range deps {
		if !d.Present {
			m = append(m, d)
		}
	}
	return m
}

// InstallHint returns a best-effort distro install command for the missing
// tool names (pacman/apt/dnf), or empty if unknown.
func InstallHint(depName string) string {
	// Detect package manager.
	var pm string
	for _, cand := range []string{"pacman", "apt-get", "dnf", "apk"} {
		if _, err := exec.LookPath(cand); err == nil {
			pm = cand
			break
		}
	}

	pkg := map[string]string{
		"git":    "git",
		"curl":   "curl",
		"tar":    "tar",
		"docker": "docker",
	}[depName]
	if pkg == "" {
		return ""
	}

	switch pm {
	case "pacman":
		return fmt.Sprintf("sudo pacman -S --needed %s", pkg)
	case "apt-get":
		return fmt.Sprintf("sudo apt-get install -y %s", pkg)
	case "dnf":
		return fmt.Sprintf("sudo dnf install -y %s", pkg)
	case "apk":
		return fmt.Sprintf("apk add %s", pkg)
	}
	return ""
}

// ---------------------------------------------------------------------------
// Capability preflight (v0.3.2 redesign).
//
// The old preflight only checked that the docker BINARY exists on PATH
// (LookPath) — it never tested the docker socket, so a user not in the docker
// group sailed through preflight and died at compose-up with "permission
// denied while trying to connect to the docker API". RunPreflight probes real
// capabilities instead: socket + daemon reachability, compose version, and
// whether the signature ports are free.
// ---------------------------------------------------------------------------

// lookPath and runCommand are the exec indirection points for the capability
// probes. Tests override them to make the probes deterministic without a real
// docker daemon. (Named runCommand rather than execCommand: execCommand is
// already taken by exec.go.)
var (
	lookPath   = exec.LookPath
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, name, args...).Output()
		return string(out), err
	}
)

// PreflightResult is the outcome of the capability probes RunPreflight runs.
type PreflightResult struct {
	DockerAccess    bool   // true if `docker info` succeeds (socket + daemon reachable)
	ComposeOK       bool   // true if `docker compose version` reports >= 2.20 (include: support)
	ComposeVersion  string // raw `docker compose version` output, when it ran
	PortsFree       bool   // ports 4858 and 4859 not currently bound
	DockerGroupHint string // actionable hint when docker access is denied
}

// RunPreflight probes the box for the capabilities the docker-bubble install
// needs. It never fails — a box missing everything is still a valid answer —
// and always returns a fully populated PreflightResult.
func RunPreflight() PreflightResult {
	var r PreflightResult

	// Docker probe: binary presence first, then the real capability (socket +
	// daemon). `docker info` with a short timeout so a hung daemon cannot
	// stall the CLI.
	if _, err := lookPath("docker"); err != nil {
		r.DockerGroupHint = "docker not found in PATH — install docker first: https://docs.docker.com/engine/install/"
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := runCommand(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	if err == nil && strings.TrimSpace(out) != "" {
		r.DockerAccess = true
	} else {
		// Binary present but the socket/daemon is unreachable. On a real
		// server the usual cause is the user not being in the docker group
		// (or not having re-logged in since being added).
		r.DockerGroupHint = "sudo usermod -aG docker $USER && re-login"
	}

	// Compose probe: version gate for `include:` support (Compose v2.20+,
	// Oct 2023). Version output looks like "Docker Compose version v2.20.0"
	// or "Docker Compose version 5.3.1".
	if out, err := runCommand(ctx, "docker", "compose", "version"); err == nil {
		r.ComposeVersion = strings.TrimSpace(out)
		if major, minor, ok := parseComposeVersion(out); ok && composeAtLeast(major, minor) {
			r.ComposeOK = true
		}
	}

	// Port probe: the signature ports must be free for the bubble to bind.
	r.PortsFree = portFree("127.0.0.1:"+ServerPort) && portFree(":"+DashboardPort)

	return r
}

// parseComposeVersion extracts major.minor from `docker compose version`
// output. Accepted forms: "Docker Compose version v2.20.0", "Docker Compose
// version 5.3.1", "Docker Compose version v2.24.2-desktop.1".
func parseComposeVersion(out string) (major, minor int, ok bool) {
	lower := strings.ToLower(out)
	idx := strings.Index(lower, "version")
	if idx < 0 {
		return 0, 0, false
	}
	fields := strings.Fields(out[idx+len("version"):])
	if len(fields) == 0 {
		return 0, 0, false
	}
	// Strip a leading "v" and any build suffix ("-desktop.1", "-rc.1", ...).
	v := strings.TrimPrefix(fields[0], "v")
	v = strings.SplitN(v, "-", 2)[0]
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// composeAtLeast reports whether the parsed compose version satisfies the
// `include:` requirement (>= 2.20).
func composeAtLeast(major, minor int) bool {
	return major > 2 || (major == 2 && minor >= 20)
}

// portFree reports whether nothing is listening on addr. A successful listen
// means the port is free (we close it again immediately); "address already in
// use" means it is not.
func portFree(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
