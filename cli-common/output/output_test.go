package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// Envelope field names are part of the public contract: this test locks the
// shape, and any field rename fails here.
func TestSuccess_EnvelopeFieldShape(t *testing.T) {
	var buf bytes.Buffer
	if err := Success(&buf, map[string]any{"id": 1}, Meta{DryRun: true}); err != nil {
		t.Fatalf("Success failed: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"ok", "data", "error", "meta"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("envelope is missing field %q: %s", key, buf.String())
		}
	}
	meta := raw["meta"].(map[string]any)
	if meta["schema_version"] != "v1" {
		t.Fatalf("schema_version should be v1: %s", buf.String())
	}
	if meta["dry_run"] != true {
		t.Fatalf("dry_run should be passed through: %s", buf.String())
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatal("output should be valid JSON")
	}
}

func TestFailure_StillEmitsParseableEnvelope(t *testing.T) {
	appErr := NewError("conflict", "The object was modified by someone else", "Re-read the latest version first", ExitConflict)
	appErr.Detail.Context = map[string]any{"code": "grp-1", "current_version": "abc123"}
	appErr.Detail.NextCommands = []string{"mytool group get grp-1 --json"}
	var buf bytes.Buffer
	if err := Failure(&buf, appErr); err != nil {
		t.Fatalf("Failure failed: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("failure output is not valid JSON: %v\n%s", err, buf.String())
	}
	if env.OK {
		t.Fatal("failure envelope ok should be false")
	}
	if env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("error code should be preserved: %s", buf.String())
	}
	if env.Error.Context["code"] != "grp-1" {
		t.Fatalf("context should be parseable: %s", buf.String())
	}
	if len(env.Error.NextCommands) != 1 {
		t.Fatalf("next_commands should be parseable: %s", buf.String())
	}
}

func TestExitCodeConstants(t *testing.T) {
	got := []int{ExitOK, ExitGeneral, ExitUsage, ExitNotFound, ExitAuth, ExitConflict, ExitTimeout}
	want := []int{0, 1, 2, 3, 4, 5, 6}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("exit code constants drifted: got %v want %v", got, want)
		}
	}
}

func TestAppError_ErrorCarriesMessage(t *testing.T) {
	appErr := NewError("usage", "missing --version", "", ExitUsage)
	if msg := appErr.Error(); msg != "missing --version" {
		t.Fatalf("Error() should return the message: %q", msg)
	}
	if errors.Unwrap(appErr) != nil {
		t.Fatal("AppError should not wrap an underlying error (exit semantics are independent)")
	}
}
