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
)

const (
	initialScannerBufferSize = 4096
	maxScannerBufferSize     = 10 * 1024 * 1024
)

type StreamHandler = func(line string, isErr bool)

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

	err := c.execute(ctx, func(line string, isErr bool) {
		buf.WriteString(line)
		buf.WriteString("\n")

		for _, h := range handlers {
			if h != nil {
				h(line, isErr)
			}
		}
	})

	return buf.String(), err
}

func (c *Command) execute(ctx context.Context, onStream StreamHandler) error {
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
		if err := c.streamOutput(stdout, onStream, false); err != nil {
			errChan <- fmt.Errorf("stdout stream error: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := c.streamOutput(stderr, onStream, true); err != nil {
			errChan <- fmt.Errorf("stderr stream error: %w", err)
		}
	}()

	wg.Wait()
	close(errChan)

	var streamErrs []error
	for err := range errChan {
		streamErrs = append(streamErrs, err)
	}

	cmdErr := cmd.Wait()

	if cmdErr != nil {
		return fmt.Errorf("command failed: %w", cmdErr)
	}

	if len(streamErrs) > 0 {
		return fmt.Errorf("stream errors occurred: %v", streamErrs)
	}

	return nil
}

func (c *Command) streamOutput(r io.Reader, handler StreamHandler, isErr bool) error {
	if handler == nil {
		handler = func(string, bool) {}
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, initialScannerBufferSize), maxScannerBufferSize)

	for scanner.Scan() {
		text := scanner.Text()
		lines := c.normalizeAndSplitLines(text)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				handler(line, isErr)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		handler(fmt.Sprintf("scanner error: %v", err), true)
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

func (c *Command) normalizeAndSplitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	return strings.Split(text, "\n")
}
