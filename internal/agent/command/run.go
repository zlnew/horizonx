// Package command
package command

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"horizonx/internal/domain"
)

const (
	initialScannerBufferSize = 4096
	maxScannerBufferSize     = 10 * 1024 * 1024
)

type StreamHandler = func(line string, stream domain.LogStream, level domain.LogLevel)

type Command struct {
	workDir string
	name    string
	args    []string
}

func NewCommand(workDir, name string, args ...string) *Command {
	return &Command{
		workDir: workDir,
		name:    name,
		args:    args,
	}
}

func (c *Command) Run(ctx context.Context, handlers ...StreamHandler) (string, error) {
	var buf bytes.Buffer

	err := c.execute(ctx, func(line string, stream domain.LogStream, level domain.LogLevel) {
		buf.WriteString(line)
		buf.WriteString("\n")

		for _, h := range handlers {
			if h != nil {
				h(line, stream, level)
			}
		}
	})

	return buf.String(), err
}

// Stream runs the command and streams each line to the handler WITHOUT
// accumulating output. Run() buffers everything for bounded commands; a
// `--follow` tail running for an hour must never buffer — that's the
// unbounded-memory regression this sibling avoids.
func (c *Command) Stream(ctx context.Context, handler StreamHandler) error {
	return c.execute(ctx, handler)
}

func (c *Command) execute(ctx context.Context, handler StreamHandler) error {
	cmd := exec.CommandContext(ctx, c.name, c.args...)
	cmd.Dir = c.workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	errChan := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := c.streamOutput(stdout, handler, domain.StreamStdout); err != nil {
			errChan <- fmt.Errorf("stdout stream error: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := c.streamOutput(stderr, handler, domain.StreamStderr); err != nil {
			errChan <- fmt.Errorf("stderr stream error: %w", err)
		}
	}()

	wg.Wait()

	cmdErr := cmd.Wait()
	close(errChan)

	var streamErrs []error
	for err := range errChan {
		streamErrs = append(streamErrs, err)
	}

	if cmdErr != nil {
		return fmt.Errorf("command failed: %w", cmdErr)
	}

	if len(streamErrs) > 0 {
		return fmt.Errorf("stream errors occurred: %v", streamErrs)
	}

	return nil
}

func (c *Command) streamOutput(r io.Reader, handler StreamHandler, stream domain.LogStream) error {
	if handler == nil {
		handler = func(string, domain.LogStream, domain.LogLevel) {}
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, initialScannerBufferSize), maxScannerBufferSize)

	for scanner.Scan() {
		text := scanner.Text()
		lines := c.normalizeAndSplitLines(text)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			level := c.classifyLine(line)
			if line != "" {
				handler(line, stream, level)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

func (c *Command) normalizeAndSplitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	return strings.Split(text, "\n")
}

func (c *Command) classifyLine(line string) domain.LogLevel {
	l := strings.ToLower(line)

	switch {
	case strings.Contains(l, "panic"),
		strings.Contains(l, "fatal"):
		return domain.LogFatal

	case strings.Contains(l, "error"),
		strings.Contains(l, "failed"),
		strings.Contains(l, "exception"):
		return domain.LogError

	case strings.Contains(l, "warn"),
		strings.Contains(l, "deprecated"):
		return domain.LogWarn

	case strings.Contains(l, "debug"),
		strings.Contains(l, "trace"):
		return domain.LogDebug
	}

	return domain.LogInfo
}
