package appframe

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
)

// newRoot builds a test root command with a persistent --json flag.
func newRoot() *cobra.Command {
	root := &cobra.Command{Use: "testcli", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("json", false, "output a stable JSON envelope")
	root.AddCommand(&cobra.Command{
		Use: "ok", RunE: func(cmd *cobra.Command, args []string) error {
			return Respond(cmd, map[string]any{"v": 1}, "human-readable output", output.Meta{})
		},
	})
	return root
}

func TestExecute_Success(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(newRoot(), []string{"ok"}, &stdout, &stderr)
	if code != output.ExitOK {
		t.Fatalf("success should return 0, got %d", code)
	}
	// Non-TTY (bytes.Buffer) automatically selects machine mode.
	if !strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("wrong machine-mode envelope: %s", stdout.String())
	}
}

func TestExecute_ExplicitJSONFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(newRoot(), []string{"--json", "ok"}, &stdout, &stderr)
	if code != output.ExitOK {
		t.Fatalf("wrong exit code: %d", code)
	}
	if !strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("explicit --json should select machine mode: %s", stdout.String())
	}
}

func TestExecute_CommandError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(newRoot(), []string{"--json", "nope"}, &stdout, &stderr)
	if code != output.ExitUsage {
		t.Fatalf("unknown command should map to exit code 2, got %d", code)
	}
	var env struct {
		Error *struct {
			Code         string         `json:"code"`
			Message      string         `json:"message"`
			Hint         string         `json:"hint"`
			NextCommands []string       `json:"next_commands"`
			Context      map[string]any `json:"context"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("machine-mode error envelope is not JSON: %s (%v)", stdout.String(), err)
	}
	if env.Error.Code != "unknown_command" {
		t.Fatalf("error code should be unknown_command: %s", stdout.String())
	}
	if env.Error.Message != `Unknown command "nope"` {
		t.Fatalf("message should carry the mistyped command name: %s", env.Error.Message)
	}
	if env.Error.Hint != "Run testcli --help to list all available commands" {
		t.Fatalf("hint should point at the root command's --help: %s", env.Error.Hint)
	}
	if len(env.Error.NextCommands) != 1 || env.Error.NextCommands[0] != "testcli --help" {
		t.Fatalf("next_commands should give a copy-pasteable command: %v", env.Error.NextCommands)
	}
	if env.Error.Context["unknown_command"] != "nope" {
		t.Fatalf("context should echo the mistyped command name: %v", env.Error.Context)
	}
	cmds, _ := env.Error.Context["available_commands"].([]any)
	if len(cmds) != 1 || cmds[0] != "ok" {
		t.Fatalf("context should enumerate available commands (registration order): %v", env.Error.Context)
	}
	if stderr.Len() != 0 {
		t.Fatalf("machine mode should not write to stderr: %s", stderr.String())
	}
}

// TestExecute_UnknownCommandSuggestion locks in the suggestion contract: a
// close mistype reuses cobra's built-in suggestions (Levenshtein/prefix),
// and the message appends "(closest: …)".
func TestExecute_UnknownCommandSuggestion(t *testing.T) {
	root := newRoot()
	root.AddCommand(&cobra.Command{
		Use: "query", RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	var stdout, stderr bytes.Buffer
	code := Execute(root, []string{"--json", "quer"}, &stdout, &stderr)
	if code != output.ExitUsage {
		t.Fatalf("unknown command should exit with code 2, got %d", code)
	}
	var env struct {
		Error *struct {
			Message string         `json:"message"`
			Context map[string]any `json:"context"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("error envelope is not JSON: %s (%v)", stdout.String(), err)
	}
	if env.Error.Message != `Unknown command "quer" (closest: query)` {
		t.Fatalf("message should carry the closest suggestion: %s", env.Error.Message)
	}
	cmds, _ := env.Error.Context["available_commands"].([]any)
	if len(cmds) != 2 || cmds[0] != "ok" || cmds[1] != "query" {
		t.Fatalf("available_commands should enumerate in registration order: %v", env.Error.Context)
	}
}

// TestExecute_UnknownSubcommandWithNoArgs locks in the NoArgs failure at the
// subcommand level: available_commands enumerates that level's subcommands,
// and the hint points at that level's --help.
func TestExecute_UnknownSubcommandWithNoArgs(t *testing.T) {
	root := newRoot()
	grp := &cobra.Command{Use: "grp"}
	grp.AddCommand(&cobra.Command{
		Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	root.AddCommand(grp)
	var stdout, stderr bytes.Buffer
	code := Execute(root, []string{"--json", "grp", "list", "foo"}, &stdout, &stderr)
	if code != output.ExitUsage {
		t.Fatalf("NoArgs with extra arguments should exit with code 2, got %d", code)
	}
	var env struct {
		Error *struct {
			Message string         `json:"message"`
			Hint    string         `json:"hint"`
			Context map[string]any `json:"context"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("error envelope is not JSON: %s (%v)", stdout.String(), err)
	}
	if env.Error.Message != `Unknown command "foo"` {
		t.Fatalf("message should carry the mistyped command name: %s", env.Error.Message)
	}
	if env.Error.Hint != "Run testcli grp list --help to list all available commands" {
		t.Fatalf("hint should point at the reporting level's --help: %s", env.Error.Hint)
	}
	if env.Error.Context["unknown_command"] != "foo" {
		t.Fatalf("context should echo the mistyped command name: %v", env.Error.Context)
	}
}

// TestAsAppError_UnknownCommandWithoutRoot locks in the path without root
// context (mytool's local funnel): the unknown_command/exit-code-2 contract
// holds, but available_commands is not attached.
func TestAsAppError_UnknownCommandWithoutRoot(t *testing.T) {
	got := AsAppError(errors.New("unknown command \"quer\" for \"testcli\"\n\nDid you mean this?\n\tquery\n"))
	if got.Detail.Code != "unknown_command" || got.Exit != output.ExitUsage {
		t.Fatalf("unknown-command classification failed: %+v exit code %d", got.Detail, got.Exit)
	}
	if got.Detail.Message != `Unknown command "quer" (closest: query)` {
		t.Fatalf("message should carry the suggestion: %s", got.Detail.Message)
	}
	if got.Detail.Hint != "Run testcli --help to list all available commands" {
		t.Fatalf("wrong hint: %s", got.Detail.Hint)
	}
	if _, has := got.Detail.Context["available_commands"]; has {
		t.Fatalf("without root context, available_commands must not be attached: %v", got.Detail.Context)
	}
	if got.Detail.Context["unknown_command"] != "quer" {
		t.Fatalf("context should echo the mistyped command name: %v", got.Detail.Context)
	}
}

// TestAvailableCommands_SkipsFrameworkAndAnnotatesAliases locks in the
// command-surface enumeration: cobra's auto-injected help/completion are
// filtered out, registration order is preserved, and aliases are annotated.
func TestAvailableCommands_SkipsFrameworkAndAnnotatesAliases(t *testing.T) {
	root := newRoot()
	root.AddCommand(&cobra.Command{
		Use: "pod", Aliases: []string{"pods"}, RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	// Simulate the framework commands cobra auto-injects on Execute
	// (injected at the end).
	root.AddCommand(&cobra.Command{Use: "help"}, &cobra.Command{Use: "completion"})
	got := availableCommands(root)
	want := []string{"ok", "pod (aliases: pods)"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("available_commands = %v, want %v", got, want)
	}
}

func TestExecute_HumanMode(t *testing.T) {
	// A TTY cannot be faked in tests, so verify the pure-function behavior
	// of AsAppError/Respond instead.
	root := newRoot()
	// No --json set and the stream is a buffer (non-TTY) →
	// CommandMachineOutput is true.
	cmd, _, _ := root.Find([]string{"ok"})
	if !CommandMachineOutput(cmd) {
		t.Fatal("non-TTY should default to machine mode")
	}
}

func TestAsAppError_AppErrorPassthrough(t *testing.T) {
	appErr := output.NewError("conflict", "conflict occurred", "hint", output.ExitConflict)
	got := AsAppError(appErr)
	if got != appErr {
		t.Fatal("AppError should be returned as-is")
	}
}

func TestAsAppError_UsageError(t *testing.T) {
	got := AsAppError(errors.New("unknown flag: --foo"))
	if got.Detail.Code != "invalid_usage" || got.Exit != output.ExitUsage {
		t.Fatalf("usage-error classification failed: %+v", got.Detail)
	}
}

func TestAsAppError_Generic(t *testing.T) {
	got := AsAppError(errors.New("network exploded"))
	if got.Detail.Code != "operation_failed" || got.Exit != output.ExitGeneral {
		t.Fatalf("generic error classification failed: %+v", got.Detail)
	}
}

func TestCommandErrorHelpers(t *testing.T) {
	err := CommandErrorWithNext("confirmation_required", "message", "hint", output.ExitUsage, "mctest get --id 1")
	appErr, ok := err.(*output.AppError)
	if !ok {
		t.Fatalf("expected an AppError, got %T", err)
	}
	if len(appErr.Detail.NextCommands) != 1 || appErr.Detail.NextCommands[0] != "mctest get --id 1" {
		t.Fatalf("wrong next_commands: %v", appErr.Detail.NextCommands)
	}
	err = CommandErrorWithCtx("conflict", "message", "hint", output.ExitConflict, []string{"mctest get"}, map[string]any{"id": 7})
	appErr = err.(*output.AppError)
	if appErr.Detail.Context["id"] != 7 {
		t.Fatalf("wrong context: %v", appErr.Detail.Context)
	}
	if appErr.Detail.Code != "conflict" || appErr.Exit != output.ExitConflict {
		t.Fatalf("wrong error code/exit code: %+v", appErr.Detail)
	}
}
