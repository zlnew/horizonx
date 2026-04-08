// Package git
package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"horizonx/internal/agent/command"
)

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) CloneOrPull(ctx context.Context, workDir string, remoteURL, branch string, handlers ...command.StreamHandler) (string, error) {
	if yes := m.IsGitRepo(workDir); yes {
		return m.Pull(ctx, workDir, branch, handlers...)
	}

	return m.Clone(ctx, workDir, remoteURL, branch, handlers...)
}

func (m *Manager) Clone(ctx context.Context, workDir string, remoteURL, branch string, handlers ...command.StreamHandler) (string, error) {
	args := []string{"clone", "--branch", branch, "--depth", "1", remoteURL, workDir}

	cmd := command.NewCommand(workDir, "git", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) Pull(ctx context.Context, workDir string, branch string, handlers ...command.StreamHandler) (string, error) {
	checkout := command.NewCommand(workDir, "git", "checkout", branch)
	if output, err := checkout.Run(ctx, handlers...); err != nil {
		return output, err
	}

	pull := command.NewCommand(workDir, "git", "pull", "origin", branch)
	return pull.Run(ctx, handlers...)
}

func (m *Manager) GetCurrentCommit(ctx context.Context, workDir string, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(workDir, "git", "rev-parse", "HEAD")

	out, err := cmd.Run(ctx, handlers...)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

func (m *Manager) GetCommitMessage(ctx context.Context, workDir string, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(workDir, "git", "log", "-1", "--pretty=%B")
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) IsGitRepo(dir string) bool {
	gitDir := filepath.Join(dir, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
}

func (m *Manager) IsGitInstalled() bool {
	cmd := exec.Command("git", "--version")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}
