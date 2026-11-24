// Package prompt provides unified reading of sensitive input such as
// passwords: environment variables take priority, then hidden terminal input;
// in a non-interactive environment without a provided value a sentinel error
// is returned, which each CLI maps to its own structured error (including the
// "ask the user once" guidance). Passwords never enter command-line arguments
// or shell history.
package prompt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
)

// ErrNeedPassword indicates that no password was provided via environment
// variables and the current environment is non-interactive, so the password
// cannot be read. Callers should map it to a structured error with "ask the
// user once" guidance.
var ErrNeedPassword = errors.New(msgNeedPasswordNonInteractive)

// PasswordOptions controls password reading behavior.
type PasswordOptions struct {
	EnvVars   []string  // environment variable names checked in order, canonical name first, compatibility aliases after
	Prompt    string    // prompt text shown before hidden input (written to Stderr)
	Stdin     *os.File  // standard input; os.Stdin is used when nil
	Stderr    io.Writer // prompt output; os.Stderr is used when nil
	Stdout    io.Writer // when non-nil, it must also be an interactive terminal before hidden input starts (some CLIs check stdout too)
	TrimSpace bool      // whether to trim surrounding whitespace before returning
}

// ReadPassword reads a password: environment variables take priority (the
// first non-empty one wins); hidden input is used on an interactive terminal;
// otherwise ErrNeedPassword is returned.
func ReadPassword(opts PasswordOptions) (string, error) {
	for _, name := range opts.EnvVars {
		if value := os.Getenv(name); value != "" {
			return value, nil
		}
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	// stdin is not a terminal → non-interactive environment (pipe/CI); we
	// cannot prompt for input.
	if !term.IsTerminal(int(stdin.Fd())) {
		return "", ErrNeedPassword
	}
	// Require stdout to be a terminal as well (the existing behavior of some
	// CLIs), to avoid polluting --json output.
	if opts.Stdout != nil && !output.IsTTY(opts.Stdout) {
		return "", ErrNeedPassword
	}
	fmt.Fprint(stderr, opts.Prompt)
	value, err := term.ReadPassword(int(stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", fmt.Errorf(msgReadPasswordFailed, err)
	}
	if opts.TrimSpace {
		return strings.TrimSpace(string(value)), nil
	}
	return string(value), nil
}
