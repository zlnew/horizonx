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

	"horizonx/internal/agent/command"
)

type Manager struct{}

type Container struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	Ports    string `json:"Ports"`
	Project  string `json:"Project"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) ComposeUp(ctx context.Context, workDir string, detached, build bool, handlers ...command.StreamHandler) (string, error) {
	args := []string{"compose", "up"}
	if detached {
		args = append(args, "-d")
	}
	if build {
		args = append(args, "--build")
	}

	cmd := command.NewCommand(workDir, "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeDown(ctx context.Context, workDir string, removeVolumes bool, handlers ...command.StreamHandler) (string, error) {
	args := []string{"compose", "down"}
	if removeVolumes {
		args = append(args, "-v")
	}

	cmd := command.NewCommand(workDir, "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeStop(ctx context.Context, workDir string, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(workDir, "docker", "compose", "stop")
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeStart(ctx context.Context, workDir string, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(workDir, "docker", "compose", "start")
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeRestart(ctx context.Context, workDir string, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(workDir, "docker", "compose", "restart")
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposeLogs(ctx context.Context, workDir string, tail int, handlers ...command.StreamHandler) (string, error) {
	args := []string{"compose", "logs"}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}

	cmd := command.NewCommand(workDir, "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ComposePs(ctx context.Context, workDir string, json bool, handlers ...command.StreamHandler) (string, error) {
	args := []string{"compose", "ps"}
	if json {
		args = append(args, "--format", "json")
	}

	cmd := command.NewCommand(workDir, "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) ValidateDockerComposeFile(workDir string) error {
	files := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, f := range files {
		if _, err := os.Stat(filepath.Join(workDir, f)); err == nil {
			return nil
		}
	}

	return fmt.Errorf("no docker-compose file found")
}

func (m *Manager) WriteEnvFile(workDir string, envVars map[string]string) error {
	envPath := filepath.Join(workDir, ".env")

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
