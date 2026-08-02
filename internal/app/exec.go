// execCommand is a thin wrapper over os/exec so prompt.go and other
// helpers can run commands without importing os/exec everywhere.
package app

import "os/exec"

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
