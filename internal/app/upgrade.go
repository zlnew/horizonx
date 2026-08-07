package app

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"horizonx/internal/version"
)

const (
	githubAPI   = "https://api.github.com/repos/zlnew/horizonx/releases/latest"
	githubDL    = "https://github.com/zlnew/horizonx/releases/latest/download"
	upgradeTime = 60 * time.Second
)

// restartServiceFn is the indirection point for the unit restart, so tests
// can assert the agent/server branch without scheduling a real systemd
// restart on the box running the tests.
var restartServiceFn = restartService

// RunUpgrade updates everything HorizonX on this box to the latest release:
//  1. Self-update the CLI binary (checksum-verified swap).
//  2. Detect components and update each:
//     - server instance (/opt/horizonx): regenerate tree (keeps .env), rebuild
//     the server image from the pinned release, fetch+load the latest
//     dashboard (best-effort), health-poll.
//     - agent systemd unit: restart it (restart only — provisioning is
//     `install agent`'s job and needs the dashboard token).
//  3. Report per component; a failure in one never aborts the others.
//
// latestReleaseFn is the indirection point for the release lookup, so tests
// can fake the network (same pattern as execCompose / preflightFn).
var latestReleaseFn = latestRelease

// downloadFn, replaceBinaryFn, and execSelfFn are seams for the self-update
// pipeline so tests can exercise the swap + re-exec decision without real
// network or a process-image replace.
var downloadFn = download
var replaceBinaryFn = replaceBinary

// execSelfFn replaces the current process image with the given binary.
// Default is syscall.Exec, which never returns on success. Test seam.
var execSelfFn = func(exe string, args, env []string) error {
	return syscall.Exec(exe, args, env)
}

// envComponentsOnly is set right before re-exec so the fresh process skips
// the self-update check and goes straight to the component pass.
const envComponentsOnly = "HORIZONX_UPGRADE_COMPONENTS_ONLY"

func RunUpgrade() error {
	// Re-exec'd after a binary swap: the new code is already running, so skip
	// the update check and go straight to the component pass.
	if os.Getenv(envComponentsOnly) == "1" {
		return upgradeComponents()
	}

	fmt.Println("horizonx " + version.Version + " → checking for updates…")

	rel, err := latestReleaseFn()
	if err != nil {
		return err
	}

	tag := strings.TrimPrefix(rel.TagName, "v")
	if tag == version.Version {
		fmt.Println("already up to date (" + version.Version + ").")
	} else {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable: %w", err)
		}

		arch := runtime.GOARCH
		if arch == "amd64" {
			arch = "x86_64"
		}
		asset := fmt.Sprintf("horizonx-%s-%s.tar.gz", runtime.GOOS, arch)

		fmt.Printf("downloading %s (%s)…\n", asset, tag)

		url := fmt.Sprintf("%s/%s", githubDL, asset)
		tarball, err := downloadFn(url, asset)
		if err != nil {
			return err
		}
		defer os.Remove(tarball)

		checksumURL := fmt.Sprintf("%s/SHA256SUMS", githubDL)
		sums, err := downloadFn(checksumURL, "SHA256SUMS")
		if err != nil {
			return err
		}
		defer os.Remove(sums)

		want, err := checksumFor(sums, asset)
		if err != nil {
			return err
		}
		if err := verifySHA256(tarball, want); err != nil {
			return err
		}
		fmt.Println("checksum OK.")

		newBin, err := extractBinary(tarball, "horizonx")
		if err != nil {
			return err
		}
		defer os.Remove(newBin)

		if err := replaceBinaryFn(newBin, exe); err != nil {
			return err
		}
		fmt.Printf("updated to horizonx %s.\n", tag)

		// CRITICAL: the swap replaced the FILE, but THIS process is still
		// executing the OLD binary's code. Running the component pass here
		// would use the old detection logic — the exact bug that shipped
		// v0.3.11: the still-running v0.3.10 process restarted the agent but
		// never looked in /opt/horizonx, so server + dashboard stayed stale.
		// Re-exec the new binary so the component pass runs with NEW code.
		os.Setenv(envComponentsOnly, "1")
		if err := execSelfFn(exe, os.Args, os.Environ()); err != nil {
			return fmt.Errorf("re-exec new binary: %w", err)
		}
		return nil // unreachable when execSelfFn is syscall.Exec (image replaced)
	}

	return upgradeComponents()
}

// upgradeComponents detects what's installed on this box and updates each
// component. Runs even when the binary was already current — the instance may
// have been installed/left on an older release. Runs in a fresh process after
// a binary swap (see RunUpgrade), so it always executes the newest code.
func upgradeComponents() error {
	rt := DetectRuntime()
	fmt.Println()
	fmt.Println("Upgrading components…")

	// Server instance: regenerate the tree (install-or-upgrade: existing .env
	// preserved by generateInstance) then apply. applyInstance pins HX_VERSION to
	// version.Version — the binary was just swapped, so this is the NEW
	// release.
	instanceDone := false
	detected := false
	if rt.InstanceInstalled() {
		detected = true
		fmt.Println("  server instance: detected at " + rt.InstanceDir)
		host := "127.0.0.1"
		// Reuse the install-server generation with defaults: the existing
		// .env (admin creds, secrets, origins) is kept verbatim by
		// generateInstance, so admin/pass/origins are irrelevant here.
		l, gerr := GenerateInstanceWithAdmin(rt.InstanceDir, host, "", "", nil)
		if gerr != nil {
			fmt.Printf("  ⚠ server instance: regenerate tree failed: %v\n", gerr)
		} else if aerr := applyInstance(l, InstallServerOptions{Host: host}); aerr != nil {
			fmt.Printf("  ⚠ server instance: update failed: %v\n", aerr)
		} else {
			instanceDone = true
		}
	}

	// Agent unit: restart only (provisioning is install agent's job).
	if rt.UnitActive(agentUnit) {
		detected = true
		fmt.Println("  agent: unit active, restarting…")
		if err := restartServiceFn(agentUnit, rt.IsUserUnit(agentUnit)); err != nil {
			fmt.Printf("  ⚠ agent: restart failed: %v\n", err)
		} else {
			fmt.Println("  ✔ agent restarted")
		}
	} else if !instanceDone && rt.UnitActive(serverUnit) {
		// Legacy bare-metal server under systemd (no instance): restart it so
		// the swapped binary takes effect.
		detected = true
		fmt.Println("  server (systemd): restarting…")
		if err := restartServiceFn(serverUnit, rt.IsUserUnit(serverUnit)); err != nil {
			fmt.Printf("  ⚠ server: restart failed: %v\n", err)
		} else {
			fmt.Println("  ✔ server restarted")
		}
	} else if !instanceDone && rt.ComposeFile != "" {
		// Legacy compose layout (pre-instance): force-recreate the stack.
		detected = true
		fmt.Println("  restarting docker compose stack…")
		if err := restartCompose(rt.ComposeFile); err != nil {
			fmt.Printf("  ⚠ compose stack restart failed: %v\n", err)
		} else {
			fmt.Println("  ✔ compose stack restarted")
		}
	}

	if !detected {
		fmt.Println("No HorizonX components detected on this box.")
		fmt.Println("  Control plane (server + dashboard): sudo horizonx install server")
		fmt.Println("  Agent on this host:                 sudo horizonx install agent --token <token>")
	}

	fmt.Println()
	// In the components-only child (re-exec'd after a swap), the binary stamp
	// is the NEW release — we were just updated. Otherwise the parent hit
	// "already up to date" and we reconciled an existing install.
	// version.Version already carries the "v" in release builds (stamp
	// "v0.3.12") — don't prepend another one (caught live on v0.3.12:
	// "upgraded to vv0.3.12").
	if os.Getenv(envComponentsOnly) == "1" {
		fmt.Println("✔ horizonx upgraded to " + version.Version + "; components reconciled.")
	} else {
		fmt.Println("✔ horizonx already current (" + version.Version + "); components reconciled.")
	}
	return nil
}

// restartService restarts a systemd unit, elevating via sudo when needed.
// The running process cannot cleanly systemctl-restart itself (it gets
// SIGTERM mid-run), so we spawn a detached helper that waits a beat, then
// restarts the unit, then reaps.
func restartService(unit string, userScope bool) error {
	scope := ""
	if userScope {
		scope = "--user"
	}
	unitName := fmt.Sprintf("horizonx-upgrade-restart-%d", os.Getpid())
	args := []string{"--unit=" + unitName, "--on-active=2", "--no-block"}
	args = append(args, "systemctl")
	if scope != "" {
		args = append(args, scope)
	}
	args = append(args, "restart", unit)
	if out, err := execCommand("systemd-run", args...).CombinedOutput(); err == nil {
		return nil
	} else if !strings.Contains(string(out), "systemd-run") {
		// systemd-run may be missing; fall back to at.
		if err := atRestart(unit, userScope); err != nil {
			return fmt.Errorf("restart %s: %v (run manually: sudo systemctl restart %s)", unit, err, unit)
		}
	}
	return nil
}

// atRestart schedules a one-shot restart via `at` when systemd-run is absent.
func atRestart(unit string, userScope bool) error {
	scope := ""
	if userScope {
		scope = "--user "
	}
	script := fmt.Sprintf("sleep 2; systemctl %srestart %s", scope, unit)
	cmd := execCommand("bash", "-c", fmt.Sprintf("echo %q | at now + 1 minute", script))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schedule restart via at: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// restartCompose runs docker compose up -d --force-recreate for the detected
// compose file so the new server image takes effect.
func restartCompose(composeFile string) error {
	out, err := execCommand("docker", "compose", "-f", composeFile, "up", "-d", "--force-recreate").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

type ghRelease struct {
	TagName string `json:"tag_name"`
}

func latestRelease() (*ghRelease, error) {
	client := &http.Client{Timeout: upgradeTime}
	resp, err := client.Get(githubAPI)
	if err != nil {
		return nil, fmt.Errorf("check latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("check latest release: GitHub returned %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}
	return &rel, nil
}

func download(url, name string) (string, error) {
	client := &http.Client{Timeout: upgradeTime}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", name, resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "horizonx-"+name+"-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	return tmp.Name(), nil
}

func checksumFor(sumsPath, asset string) (string, error) {
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found", asset)
}

func verifySHA256(file, wantHex string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(wantHex) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, wantHex)
	}
	return nil
}

func extractBinary(tarball, binName string) (string, error) {
	f, err := os.Open(tarball)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	// Track the first regular file as a fallback: a tarball that contains a
	// single binary under ANY name (e.g. an arch-qualified name from a broken
	// packaging run) should still upgrade cleanly.
	var fallback string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == binName {
			return writeExtracted(tr)
		}
		if fallback == "" {
			fallback = hdr.Name
		}
	}
	if fallback != "" {
		// Re-open: the tar reader is exhausted, and the fallback entry was
		// never consumed. Rewind and extract the sole file.
		f.Seek(0, io.SeekStart)
		gz.Close()
		gz, err = gzip.NewReader(f)
		if err != nil {
			return "", err
		}
		defer gz.Close()
		tr = tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			if hdr.Typeflag == tar.TypeReg && hdr.Name == fallback {
				return writeExtracted(tr)
			}
		}
	}
	return "", fmt.Errorf("%s not found in release tarball", binName)
}

// writeExtracted copies the current tar entry to a temp file and makes it
// executable. The caller is responsible for removing the returned path.
func writeExtracted(tr *tar.Reader) (string, error) {
	tmp, err := os.CreateTemp("", "horizonx-newbin-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, tr); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func replaceBinary(newBin, exe string) error {
	// Make the running binary replaceable (Linux allows rename over a
	// running file; a plain write would fail with ETXTBSY).
	backup := exe + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(exe, backup); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(newBin, exe); err != nil {
		// Attempt rollback.
		_ = os.Rename(backup, exe)
		return fmt.Errorf("install new binary: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}
