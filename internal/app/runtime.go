// Runtime detection shared by setup + upgrade.
//
// HorizonX can run as a plain binary, under systemd, or in Docker Compose.
// Instead of asking the user "systemd or docker?", we probe the box and
// recommend. setup uses this to pick a default install method; upgrade uses
// it to restart the right unit after swapping the binary.
package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runtime describes how horizonx is (or would be) installed on this box.
type Runtime struct {
	// Systemd is true when PID 1 is systemd (systemctl available).
	Systemd bool
	// DockerCLI is true when `docker` + `docker compose` are on PATH.
	DockerCLI bool
	// ComposeFile is the docker-compose file horizonx should use, if any.
	ComposeFile string
	// InstanceDir is the /opt/horizonx instance root when the install-server
	// instance tree is present (its own root compose). Distinct from
	// ComposeFile, which covers legacy compose layouts elsewhere on disk.
	InstanceDir string
	// SystemdUnits lists horizonx-*.service units present on the box.
	SystemdUnits []string
	// UserUnits lists units found in ~/.config/systemd/user (no sudo installs).
	UserUnits []string

	// Effective paths.
	BinPath string // resolved horizonx executable
	DataDir string // /var/lib/horizonx or ~/.local/share/horizonx
	EnvDir  string // /etc/horizonx or ~/.config/horizonx
}

const (
	serverUnit = "horizonx-server.service"
	agentUnit  = "horizonx-agent.service"
)

// DetectRuntime probes the box and returns what it finds. Never fails — a
// bare box is still a valid answer (plain binary install).
func DetectRuntime() *Runtime {
	rt := &Runtime{}

	// Systemd?
	if _, err := exec.LookPath("systemctl"); err == nil {
		rt.Systemd = true
		if out, err := exec.Command("systemctl", "is-system-running").Output(); err == nil {
			st := strings.TrimSpace(string(out))
			if st == "running" || st == "degraded" {
				rt.Systemd = true
			}
		}
	}

	// Docker CLI + compose plugin?
	// NOTE: do NOT match on the version string (e.g. Contains(out, "v2")) —
	// modern compose reports "Docker Compose version 5.3.1", so a major-version
	// literal false-negatives on working installs. `docker compose version`
	// succeeding already proves the v2+ plugin is present (v1 standalone
	// `docker-compose` has no `compose` subcommand and errors here).
	if _, err := exec.LookPath("docker"); err == nil {
		if out, err := exec.Command("docker", "compose", "version").CombinedOutput(); err == nil &&
			strings.Contains(strings.ToLower(string(out)), "compose version") {
			rt.DockerCLI = true
		}
	}

	// Existing systemd units?
	for _, unit := range []string{serverUnit, agentUnit} {
		if _, err := os.Stat(filepath.Join("/etc/systemd/system", unit)); err == nil {
			rt.SystemdUnits = append(rt.SystemdUnits, unit)
		}
		if _, err := os.Stat(filepath.Join("/lib/systemd/system", unit)); err == nil {
			rt.SystemdUnits = append(rt.SystemdUnits, unit)
		}
		// User-level units (~/.config/systemd/user) — the workspace installs
		// the agent as the login user without sudo.
		if home, err := os.UserHomeDir(); err == nil {
			if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", unit)); err == nil {
				rt.UserUnits = append(rt.UserUnits, unit)
			}
		}
	}

	// Existing compose file? (setup output dir / cwd)
	for _, d := range []string{".", "/etc/horizonx", "/var/lib/horizonx", "horizonx-setup"} {
		for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
			p := filepath.Join(d, name)
			if _, err := os.Stat(p); err == nil {
				rt.ComposeFile = p
				break
			}
		}
		if rt.ComposeFile != "" {
			break
		}
	}

	// The install-server instance (/opt/horizonx, or HORIZONX_PREFIX override)
	// is the modern layout — its own root compose + .env. Detection is by the
	// root compose file; InstanceInstalled() additionally requires the .env so
	// a half-generated dir is not treated as a live instance.
	for _, d := range []string{os.Getenv("HORIZONX_PREFIX"), instanceDir} {
		if d == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "docker-compose.yml")); err == nil {
			rt.InstanceDir = d
			break
		}
	}

	// Binary path (where the running executable lives).
	if exe, err := os.Executable(); err == nil {
		rt.BinPath = exe
	} else {
		rt.BinPath = "horizonx"
	}

	// Data + env dirs: root-ish installs use /var/lib + /etc; user installs use home.
	if os.Geteuid() == 0 || os.Getenv("HORIZONX_PREFIX") != "" {
		rt.DataDir = "/var/lib/horizonx"
		rt.EnvDir = "/etc/horizonx"
	} else if home, err := os.UserHomeDir(); err == nil {
		rt.DataDir = filepath.Join(home, ".local", "share", "horizonx")
		rt.EnvDir = filepath.Join(home, ".config", "horizonx")
	} else {
		rt.DataDir = "/var/lib/horizonx"
		rt.EnvDir = "/etc/horizonx"
	}

	return rt
}

// RecommendedMethod returns the install method the box points to:
//   - "systemd" when systemd is present and docker is not
//   - "docker" when docker CLI + compose are present
//   - "systemd" when neither (fall back to systemd on a bare box)
func (r *Runtime) RecommendedMethod() string {
	if r.DockerCLI && !r.Systemd {
		return "docker"
	}
	if r.DockerCLI && r.Systemd {
		// Both present: prefer systemd for server/agent (no daemon needed),
		// but docker if a compose file already exists.
		if r.ComposeFile != "" {
			return "docker"
		}
		return "systemd"
	}
	return "systemd"
}

// HasUnit reports whether a horizonx-*.service unit is installed.
func (r *Runtime) HasUnit(name string) bool {
	for _, u := range r.SystemdUnits {
		if u == name {
			return true
		}
	}
	return false
}

// UnitActive reports whether a systemd unit is currently running. Checks
// system scope first, then the user scope (~/.config/systemd/user).
func (r *Runtime) UnitActive(name string) bool {
	if !r.Systemd {
		return false
	}
	if out, err := exec.Command("systemctl", "is-active", name).Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return true
	}
	if out, err := exec.Command("systemctl", "--user", "is-active", name).Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		return true
	}
	return false
}

// ActiveUnit returns the name of a running horizonx unit, or "" if none.
// System scope is checked first, then user scope.
func (r *Runtime) ActiveUnit() string {
	for _, u := range []string{serverUnit, agentUnit} {
		if r.UnitActive(u) {
			return u
		}
	}
	return ""
}

// InstanceInstalled reports whether the install-server instance is live on this
// box: the instance root compose exists AND the .env exists (a half-generated
// dir without .env is not a real install).
func (r *Runtime) InstanceInstalled() bool {
	if r.InstanceDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(r.InstanceDir, ".env"))
	return err == nil
}

// IsUserUnit reports whether the named unit is installed in the user scope.
func (r *Runtime) IsUserUnit(name string) bool {
	for _, u := range r.UserUnits {
		if u == name {
			return true
		}
	}
	return false
}

// isRoot reports whether the process runs as uid 0.
func isRoot() bool { return os.Geteuid() == 0 }

// needsSudo reports whether a path write requires elevation (dir exists but
// is not writable by us, or is outside our home).
func needsSudo(path string) bool {
	if isRoot() {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		// Doesn't exist — try the parent.
		parent := filepath.Dir(path)
		if pfi, perr := os.Stat(parent); perr == nil {
			return pfi.Mode().Perm()&0o200 == 0
		}
		return true
	}
	return fi.Mode().Perm()&0o200 == 0
}
