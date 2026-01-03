// Package docker
package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"horizonx-server/internal/agent/command"
)

type Manager struct {
	workDir string
}

type Container struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	Ports    string `json:"Ports"`
	Project  string `json:"Project"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

func NewManager(workDir string) *Manager {
	return &Manager{workDir: workDir}
}

func (m *Manager) Initialize() error {
	if err := os.MkdirAll(m.workDir, 0o755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	return nil
}

func (m *Manager) GetAppDir(appID int64) string {
	return filepath.Join(m.workDir, fmt.Sprintf("app-%d", appID))
}

func (m *Manager) ComposeUp(ctx context.Context, appID int64, detached, build bool, handlers ...command.StreamHandler) (string, error) {
	args := []string{"compose", "up"}
	if detached {
		args = append(args, "-d")
	}
	if build {
		args = append(args, "--build")
	}

	cmd := command.NewCommand(m.GetAppDir(appID), "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeDown(ctx context.Context, appID int64, removeVolumes bool, handlers ...command.StreamHandler) (string, error) {
	args := []string{"compose", "down"}
	if removeVolumes {
		args = append(args, "-v")
	}

	cmd := command.NewCommand(m.GetAppDir(appID), "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeStop(ctx context.Context, appID int64, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(m.GetAppDir(appID), "docker", "compose", "stop")
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeStart(ctx context.Context, appID int64, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(m.GetAppDir(appID), "docker", "compose", "start")
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeRestart(ctx context.Context, appID int64, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(m.GetAppDir(appID), "docker", "compose", "restart")
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeLogs(ctx context.Context, appID int64, tail int, handlers ...command.StreamHandler) (string, error) {
	args := []string{"compose", "logs"}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}

	cmd := command.NewCommand(m.GetAppDir(appID), "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposePs(ctx context.Context, appID int64, json bool, handlers ...command.StreamHandler) (string, error) {
	args := []string{"compose", "ps"}
	if json {
		args = append(args, "--format", "json")
	}

	cmd := command.NewCommand(m.GetAppDir(appID), "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ValidateDockerComposeFile(appID int64) error {
	appDir := m.GetAppDir(appID)
	files := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, f := range files {
		if _, err := os.Stat(filepath.Join(appDir, f)); err == nil {
			return nil
		}
	}

	return fmt.Errorf("no docker-compose file found")
}

func (m *Manager) WriteEnvFile(appID int64, envVars map[string]string) error {
	appDir := m.GetAppDir(appID)
	envPath := filepath.Join(appDir, ".env")

	var buf bytes.Buffer
	for k, v := range envVars {
		v = strings.ReplaceAll(v, "\n", "\\n")
		buf.WriteString(fmt.Sprintf("%s=\"%s\"\n", k, v))
	}

	return os.WriteFile(envPath, buf.Bytes(), 0o600)
}

func (m *Manager) IsDockerInstalled() bool {
	return exec.Command("docker", "--version").Run() == nil
}

func (m *Manager) IsDockerComposeAvailable() bool {
	return exec.Command("docker", "compose", "version").Run() == nil
}
