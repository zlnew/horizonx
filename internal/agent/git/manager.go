// Package git
package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"horizonx-server/internal/agent/command"
)

type Manager struct {
	workDir string
}

func NewManager(workDir string) *Manager {
	return &Manager{workDir: workDir}
}

func (m *Manager) GetAppDir(appID int64) string {
	return filepath.Join(m.workDir, fmt.Sprintf("app-%d", appID))
}

func (m *Manager) CloneOrPull(ctx context.Context, appID int64, remoteURL, branch string, handlers ...command.StreamHandler) (string, error) {
	appDir := m.GetAppDir(appID)

	if yes := m.IsGitRepo(appDir); yes {
		return m.Pull(ctx, appID, branch, handlers...)
	}

	return m.Clone(ctx, appID, remoteURL, branch, handlers...)
}

func (m *Manager) Clone(ctx context.Context, appID int64, remoteURL, branch string, handlers ...command.StreamHandler) (string, error) {
	appDir := m.GetAppDir(appID)
	args := []string{"clone", "--branch", branch, "--depth", "1", remoteURL, appDir}

	cmd := command.NewCommand(appDir, "git", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) Pull(ctx context.Context, appID int64, branch string, handlers ...command.StreamHandler) (string, error) {
	appDir := m.GetAppDir(appID)

	checkout := command.NewCommand(appDir, "git", "checkout", branch)
	if output, err := checkout.Run(ctx, handlers...); err != nil {
		return output, err
	}

	pull := command.NewCommand(appDir, "git", "pull", "origin", branch)
	return pull.Run(ctx, handlers...)
}

func (m *Manager) GetCurrentCommit(ctx context.Context, appID int64, handlers ...command.StreamHandler) (string, error) {
	appDir := m.GetAppDir(appID)

	cmd := command.NewCommand(appDir, "git", "rev-parse", "HEAD")
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) GetCommitMessage(ctx context.Context, appID int64, handlers ...command.StreamHandler) (string, error) {
	appDir := m.GetAppDir(appID)

	cmd := command.NewCommand(appDir, "git", "log", "-1", "--pretty=%B")
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
