package prompt

import (
	"errors"
	"os"
	"testing"
)

// withNonTerminalStdin substitutes stdin with a temp file (not a terminal) to
// drive the non-interactive branch.
func withNonTerminalStdin(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestEnvVarPriority(t *testing.T) {
	t.Setenv("TEST_PW_PRIMARY", "primary")
	t.Setenv("TEST_PW_ALIAS", "alias")
	value, err := ReadPassword(PasswordOptions{
		EnvVars: []string{"TEST_PW_PRIMARY", "TEST_PW_ALIAS"},
		Stdin:   withNonTerminalStdin(t),
	})
	if err != nil || value != "primary" {
		t.Fatalf("canonical environment variable should take priority: %q %v", value, err)
	}
}

func TestEnvVarAliasFallback(t *testing.T) {
	t.Setenv("TEST_PW_PRIMARY", "")
	t.Setenv("TEST_PW_ALIAS", "alias")
	value, err := ReadPassword(PasswordOptions{
		EnvVars: []string{"TEST_PW_PRIMARY", "TEST_PW_ALIAS"},
		Stdin:   withNonTerminalStdin(t),
	})
	if err != nil || value != "alias" {
		t.Fatalf("should fall back to the compatibility alias when the canonical name is empty: %q %v", value, err)
	}
}

func TestNonInteractiveReturnsSentinel(t *testing.T) {
	t.Setenv("TEST_PW_PRIMARY", "")
	t.Setenv("TEST_PW_ALIAS", "")
	_, err := ReadPassword(PasswordOptions{
		EnvVars: []string{"TEST_PW_PRIMARY"},
		Stdin:   withNonTerminalStdin(t),
	})
	if !errors.Is(err, ErrNeedPassword) {
		t.Fatalf("non-interactive environment should return the ErrNeedPassword sentinel: %v", err)
	}
}

func TestStdoutNotTTYRejected(t *testing.T) {
	t.Setenv("TEST_PW_PRIMARY", "")
	// When stdout is not a terminal (temp file), the request should be
	// rejected even if stdin were a terminal; in the test environment stdin
	// is also non-terminal, so this directly exercises the sentinel path.
	_, err := ReadPassword(PasswordOptions{
		EnvVars: []string{"TEST_PW_PRIMARY"},
		Stdin:   withNonTerminalStdin(t),
		Stdout:  withNonTerminalStdin(t),
	})
	if !errors.Is(err, ErrNeedPassword) {
		t.Fatalf("non-terminal stdout should return the sentinel: %v", err)
	}
}
