package app

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

// withMockedExec overrides the preflight exec indirection vars for the duration
// of a test and restores the real implementations afterwards.
func withMockedExec(t *testing.T, look func(string) (string, error), run func(context.Context, string, ...string) (string, error)) {
	t.Helper()
	origLook, origRun := lookPath, runCommand
	lookPath, runCommand = look, run
	t.Cleanup(func() { lookPath, runCommand = origLook, origRun })
}

// fakeDockerOK returns a runCommand that behaves like a healthy docker CLI:
// `docker info --format {{.ServerVersion}}` succeeds, `docker compose version`
// reports the given version string.
func fakeDockerOK(composeVersion string) func(context.Context, string, ...string) (string, error) {
	return func(_ context.Context, name string, args ...string) (string, error) {
		if name != "docker" {
			return "", errors.New("unexpected command " + name)
		}
		switch args[0] {
		case "info":
			return "24.0.7\n", nil
		case "compose":
			return composeVersion, nil
		}
		return "", errors.New("unexpected args: " + strings.Join(args, " "))
	}
}

func TestRunPreflightDockerBinaryMissing(t *testing.T) {
	withMockedExec(t,
		func(string) (string, error) { return "", errors.New("executable not found") },
		func(context.Context, string, ...string) (string, error) { return "", errors.New("not reached") },
	)

	r := RunPreflight()
	if r.DockerAccess {
		t.Errorf("DockerAccess = true, want false when docker binary is missing")
	}
	if !strings.Contains(strings.ToLower(r.DockerGroupHint), "install docker") {
		t.Errorf("DockerGroupHint = %q, want a hint mentioning installing docker", r.DockerGroupHint)
	}
}

func TestRunPreflightDockerSocketDenied(t *testing.T) {
	withMockedExec(t,
		func(string) (string, error) { return "/usr/bin/docker", nil },
		func(_ context.Context, name string, args ...string) (string, error) {
			// Binary exists but the docker socket is unreachable (not in docker group).
			return "", errors.New("permission denied while trying to connect to the docker API at unix:///var/run/docker.sock")
		},
	)

	r := RunPreflight()
	if r.DockerAccess {
		t.Errorf("DockerAccess = true, want false when the docker socket is denied")
	}
	if !strings.Contains(r.DockerGroupHint, "usermod -aG docker") {
		t.Errorf("DockerGroupHint = %q, want it to contain 'usermod -aG docker'", r.DockerGroupHint)
	}
}

func TestRunPreflightDockerOKCompose531(t *testing.T) {
	withMockedExec(t,
		func(string) (string, error) { return "/usr/bin/docker", nil },
		fakeDockerOK("Docker Compose version 5.3.1"),
	)

	r := RunPreflight()
	if !r.DockerAccess {
		t.Errorf("DockerAccess = false, want true when docker info succeeds")
	}
	if !r.ComposeOK {
		t.Errorf("ComposeOK = false, want true for compose 5.3.1 (>= 2.20)")
	}
	if !strings.Contains(r.ComposeVersion, "5.3.1") {
		t.Errorf("ComposeVersion = %q, want it to contain 5.3.1", r.ComposeVersion)
	}
}

func TestRunPreflightComposeV219(t *testing.T) {
	withMockedExec(t,
		func(string) (string, error) { return "/usr/bin/docker", nil },
		fakeDockerOK("Docker Compose version v2.19.1"),
	)

	r := RunPreflight()
	if !r.DockerAccess {
		t.Errorf("DockerAccess = false, want true (docker info is fine)")
	}
	if r.ComposeOK {
		t.Errorf("ComposeOK = true, want false for compose v2.19.1 (< 2.20)")
	}
}

func TestRunPreflightComposeUnparseable(t *testing.T) {
	withMockedExec(t,
		func(string) (string, error) { return "/usr/bin/docker", nil },
		fakeDockerOK("docker: 'compose' is not a docker command\n"),
	)

	r := RunPreflight()
	if r.ComposeOK {
		t.Errorf("ComposeOK = true, want false when compose version output is unparseable")
	}
}

func TestRunPreflightComposeCommandErrors(t *testing.T) {
	withMockedExec(t,
		func(string) (string, error) { return "/usr/bin/docker", nil },
		func(_ context.Context, name string, args ...string) (string, error) {
			if name == "docker" && len(args) > 0 && args[0] == "info" {
				return "24.0.7\n", nil
			}
			return "", errors.New("exit status 1: docker: unknown command")
		},
	)

	r := RunPreflight()
	if r.ComposeOK {
		t.Errorf("ComposeOK = true, want false when `docker compose version` errors")
	}
}

func TestRunPreflightPortsFree(t *testing.T) {
	withMockedExec(t,
		func(string) (string, error) { return "/usr/bin/docker", nil },
		fakeDockerOK("Docker Compose version v2.24.0"),
	)

	r := RunPreflight()
	if !r.PortsFree {
		t.Errorf("PortsFree = false, want true when 4858/4859 are not bound")
	}
}

func TestRunPreflightPortsBound(t *testing.T) {
	// Occupy ServerPort to prove the 'address already in use' path.
	ln, err := net.Listen("tcp", "127.0.0.1:"+ServerPort)
	if err != nil {
		t.Skipf("cannot bind test port %s: %v", ServerPort, err)
	}
	defer ln.Close()

	withMockedExec(t,
		func(string) (string, error) { return "/usr/bin/docker", nil },
		fakeDockerOK("Docker Compose version v2.24.0"),
	)

	r := RunPreflight()
	if r.PortsFree {
		t.Errorf("PortsFree = true, want false when %s is already bound", ServerPort)
	}
}
