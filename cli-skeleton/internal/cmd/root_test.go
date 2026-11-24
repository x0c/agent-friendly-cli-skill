package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
)

// TestRootVersionCommandFields locks the field set of the version command
// output (template drift protection).
func TestRootVersionCommandFields(t *testing.T) {
	root := NewRootCommand()
	var stdout bytes.Buffer
	root.SetArgs([]string{"version"})
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	if err := root.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`"version"`, `"build_commit"`, `"build_time"`, `"schema_version":"v1"`,
		`"read_only":false`, `"platform"`, `"notice"`, `"capabilities"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("version output missing %s: %s", want, out)
		}
	}
}

// TestExecuteCommandErrorMapping locks the entry-point wiring: unknown
// commands map to unknown_command / exit code 2 (the shared package's contract).
func TestExecuteCommandErrorMapping(t *testing.T) {
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	code := appframe.Execute(root, []string{"--json", "nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("unknown command should exit with code 2, got %d", code)
	}
	if !strings.Contains(stdout.String(), `"code":"unknown_command"`) {
		t.Fatalf("machine-mode error envelope mismatch: %s", stdout.String())
	}
}

// TestLoadProfileEmpty locks the guidance error when no profile exists.
func TestLoadProfileEmpty(t *testing.T) {
	t.Setenv("MCSKEL_CONFIG_HOME", t.TempDir())
	_, err := loadProfile("")
	if err == nil {
		t.Fatal("expected an error when no profile exists")
	}
	t.Logf("error: %v", err)
}

// TestProfileRoundTripWithStore locks profile storage and fingerprinting (the
// on-disk format and the read-then-write version).
func TestProfileRoundTripWithStore(t *testing.T) {
	t.Setenv("MCSKEL_CONFIG_HOME", t.TempDir())
	p := &Profile{}
	p.Name, p.Server, p.Username, p.Password = "t", "https://example.com", "u", "pw"
	if err := store.Save(p, false); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := store.Load("t")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.Server != p.Server || loaded.Username != "u" {
		t.Fatalf("round-trip field mismatch: %+v", loaded)
	}
	v1, err := fp.Of(loaded)
	if err != nil {
		t.Fatal(err)
	}
	// The fingerprint must be stable for identical business fields, and must
	// change when a field changes (the read-then-write guard depends on this).
	loaded.Username = "u2"
	v2, _ := fp.Of(loaded)
	if v1 == v2 {
		t.Fatalf("fingerprint should change with business fields: %s vs %s", v1, v2)
	}
}

// TestGuardThreeBranches locks the three-branch semantics of the
// read-then-write guard (exit codes 2/5, and pass-through).
func TestGuardThreeBranches(t *testing.T) {
	target := map[string]any{"name": "t"}
	err := guard.Check("", "latest", target, "update", "mcskel profile show --name t")
	if err.(*output.AppError).Exit != 2 {
		t.Fatalf("missing version should exit with code 2")
	}
	err = guard.Check("stale", "latest", target, "update", "cmd")
	if err.(*output.AppError).Exit != 5 {
		t.Fatalf("mismatched version should exit with code 5")
	}
	if err := guard.Check("latest", "latest", target, "update", "cmd"); err != nil {
		t.Fatalf("matching version should pass: %v", err)
	}
}
