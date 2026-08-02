// Interactive prompt helpers for the setup wizard.
//
// Everything reads from an io.Reader (stdin in production, a strings.Reader
// in tests) and writes to an io.Writer (stdout). Non-interactive mode uses
// defaults/flag values instead of blocking on input, so `horizonx setup
// --method docker --mode full` works in scripts and CI.
package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Prompt holds the wizard's IO endpoints + a non-interactive flag.
type Prompt struct {
	In     io.Reader
	Out    io.Writer
	NoTTY  bool // if true, return defaults instead of asking
	sc     *bufio.Scanner
}

// NewPrompt builds a Prompt bound to the process stdin/stdout.
func NewPrompt() *Prompt {
	return &Prompt{In: os.Stdin, Out: os.Stdout}
}

// out returns the writer, defaulting to stdout when unset (tests).
func (p *Prompt) out() io.Writer {
	if p.Out == nil {
		return os.Stdout
	}
	return p.Out
}

func (p *Prompt) scan() *bufio.Scanner {
	if p.sc == nil {
		p.sc = bufio.NewScanner(p.In)
	}
	return p.sc
}

// Ask prints a question and returns the trimmed answer. In NoTTY mode it
// returns def without reading input.
func (p *Prompt) Ask(question, def string) string {
	fmt.Fprintf(p.out(), "%s", question)
	if def != "" {
		fmt.Fprintf(p.out(), " [%s]", def)
	}
	fmt.Fprint(p.out(), ": ")
	if p.NoTTY {
		fmt.Fprintln(p.out(), def)
		return def
	}
	if !p.scan().Scan() {
		fmt.Fprintln(p.out())
		return def
	}
	line := strings.TrimSpace(p.scan().Text())
	if line == "" {
		return def
	}
	return line
}

// Confirm asks a yes/no question. Returns def when non-interactive.
func (p *Prompt) Confirm(question string, def bool) bool {
	suffix := "y/N"
	if def {
		suffix = "Y/n"
	}
	fmt.Fprintf(p.out(), "%s [%s]: ", question, suffix)
	if p.NoTTY {
		fmt.Fprintln(p.out(), map[bool]string{true: "y", false: "n"}[def])
		return def
	}
	if !p.scan().Scan() {
		fmt.Fprintln(p.out())
		return def
	}
	switch strings.ToLower(strings.TrimSpace(p.scan().Text())) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

// Choose prints a numbered menu and returns the selected index (0-based).
// In NoTTY mode it returns the default index.
func (p *Prompt) Choose(question string, options []string, def int) int {
	fmt.Fprintf(p.out(), "%s\n", question)
	for i, opt := range options {
		mark := " "
		if i == def {
			mark = ">"
		}
		fmt.Fprintf(p.out(), "  %s %d. %s\n", mark, i+1, opt)
	}
	fmt.Fprint(p.out(), "Select (number): ")
	if p.NoTTY {
		fmt.Fprintln(p.out(), def+1)
		return def
	}
	if !p.scan().Scan() {
		fmt.Fprintln(p.out())
		return def
	}
	line := strings.TrimSpace(p.scan().Text())
	if n, err := strconvAtoi(line); err == nil && n >= 1 && n <= len(options) {
		return n - 1
	}
	return def
}

// Password asks for a secret without echoing (best-effort; falls back to
// plain read when the terminal doesn't support it).
func (p *Prompt) Password(question string) string {
	fmt.Fprintf(p.out(), "%s: ", question)
	if p.NoTTY {
		fmt.Fprintln(p.out(), "(generated)")
		return ""
	}
	// Best-effort: disable echo via stty; restore on return.
	term := "/dev/tty"
	f, err := os.OpenFile(term, os.O_RDWR, 0)
	if err != nil {
		// Fall back to stdin, echo visible (tests, non-tty).
		if !p.scan().Scan() {
			fmt.Fprintln(p.out())
			return ""
		}
		v := strings.TrimSpace(p.scan().Text())
		fmt.Fprintln(p.out())
		return v
	}
	defer f.Close()

	// Try stty -echo; ignore failures (e.g. non-tty).
	_ = runSilent("stty", "-echo")
	defer func() { _ = runSilent("stty", "echo") }()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		fmt.Fprintln(p.out())
		return ""
	}
	v := strings.TrimSpace(sc.Text())
	fmt.Fprintln(p.out())
	return v
}

// Section prints a wizard section header.
func (p *Prompt) Section(title string) {
	fmt.Fprintf(p.out(), "\n── %s ──\n", title)
}

// Info prints an indented note.
func (p *Prompt) Info(format string, args ...any) {
	fmt.Fprintf(p.out(), "   "+format+"\n", args...)
}

func strconvAtoi(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func runSilent(name string, args ...string) error {
	return execCommand(name, args...).Run()
}
