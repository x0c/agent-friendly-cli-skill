package writeguard

import (
	"errors"
	"testing"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
)

// testGuard builds a test guard with the lowercase error-code style.
func testGuard() Guard {
	return Guard{
		CodeMissing:  "confirmation_required",
		CodeMismatch: "conflict",
		ExitMissing:  output.ExitUsage,
		ExitMismatch: output.ExitConflict,
		MsgMissing:   "Blocked this {action}: you have not read the object first, and no --version token was provided.",
		MsgMismatch:  "Blocked this {action}: the version does not match; the state you hold is not the latest for this object.",
		HintMissing:  "Read the object first, then retry with its version field passed as --version.",
		HintMismatch: "Read the object again and retry with the latest version.",
	}
}

func TestCheck_Missing(t *testing.T) {
	err := testGuard().Check("", "abc123", map[string]any{"id": 7}, "modify the producer", "mytest producer get --id 7")
	appErr, ok := err.(*output.AppError)
	if !ok {
		t.Fatalf("expected an AppError: %T", err)
	}
	if appErr.Detail.Code != "confirmation_required" || appErr.Exit != output.ExitUsage {
		t.Fatalf("wrong error code/exit code: %+v", appErr.Detail)
	}
	if appErr.Detail.Message != "Blocked this modify the producer: you have not read the object first, and no --version token was provided." {
		t.Fatalf("message template expanded incorrectly: %s", appErr.Detail.Message)
	}
	if appErr.Detail.Context["current_version"] != "abc123" {
		t.Fatalf("current_version missing: %v", appErr.Detail.Context)
	}
	if appErr.Detail.Context["target"] == nil {
		t.Fatalf("target missing")
	}
	if len(appErr.Detail.NextCommands) != 1 || appErr.Detail.NextCommands[0] != "mytest producer get --id 7" {
		t.Fatalf("wrong next_commands: %v", appErr.Detail.NextCommands)
	}
}

func TestCheck_Mismatch(t *testing.T) {
	err := testGuard().Check("stale00", "abc123", map[string]any{"id": 7}, "approve", "mytest get --id 7")
	appErr := err.(*output.AppError)
	if appErr.Detail.Code != "conflict" || appErr.Exit != output.ExitConflict {
		t.Fatalf("wrong error code/exit code: %+v", appErr.Detail)
	}
	if appErr.Detail.Context["version_you_had"] != "stale00" || appErr.Detail.Context["version_latest"] != "abc123" {
		t.Fatalf("wrong version context: %v", appErr.Detail.Context)
	}
}

func TestCheck_Match(t *testing.T) {
	if err := testGuard().Check("abc123", "abc123", map[string]any{"id": 7}, "modify", "mytest get --id 7"); err != nil {
		t.Fatalf("a matching version should pass: %v", err)
	}
}

func TestCheck_OldStyleUppercaseCodes(t *testing.T) {
	// Injection of old-style uppercase error codes (locked by compat tests;
	// do not unify opportunistically).
	g := testGuard()
	g.CodeMissing, g.CodeMismatch = "CONFIRMATION_REQUIRED", "CONFLICT"
	err := g.Check("", "x", nil, "modify", "cmd")
	if err.(*output.AppError).Detail.Code != "CONFIRMATION_REQUIRED" {
		t.Fatalf("uppercase error-code injection failed: %v", err)
	}
}

func TestCheck_NoActionPlaceholder(t *testing.T) {
	// A template without the {action} placeholder must be emitted verbatim.
	g := testGuard()
	g.MsgMissing = "fixed message"
	err := g.Check("", "x", nil, "any action", "cmd")
	if err.(*output.AppError).Detail.Message != "fixed message" {
		t.Fatalf("a placeholder-free template must be emitted verbatim: %s", err.(*output.AppError).Detail.Message)
	}
}

func TestCheck_ErrorsIs(t *testing.T) {
	err := testGuard().Check("", "x", nil, "modify", "cmd")
	if !errors.Is(err, err) {
		t.Fatal("errors.Is with itself must hold")
	}
}
