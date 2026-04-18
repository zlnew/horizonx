// Package docker
package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func (m *Manager) Cmd(ctx context.Context, workDir string, args []string, handlers ...command.StreamHandler) (string, error) {
	cmd := command.NewCommand(workDir, "docker", args...)
	return cmd.Run(ctx, handlers...)
}

func (m *Manager) GetDockerComposeFile(workDir string) (string, error) {
	files := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, f := range files {
		path := filepath.Join(workDir, f)

		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("docker compose file not found")
}

func (m *Manager) GetDockerfile(workDir string) (string, error) {
	files := []string{
		"docker/Dockerfile",
		"Dockerfile",
	}

	for _, f := range files {
		path := filepath.Join(workDir, f)

		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("Dockerfile not found")
}

func (m *Manager) WriteEnvFile(workDir string, envVars map[string]string) error {
	envPath := filepath.Join(workDir, ".env")

	var buf bytes.Buffer
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := strings.ReplaceAll(envVars[k], "\n", "\\n")
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
