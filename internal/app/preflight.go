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
		"git":   "git",
		"curl":  "curl",
		"tar":   "tar",
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
