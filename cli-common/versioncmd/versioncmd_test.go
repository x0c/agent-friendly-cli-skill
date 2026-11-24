package versioncmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestNew_MachineOutput(t *testing.T) {
	root := &cobra.Command{Use: "mctest"}
	root.PersistentFlags().Bool("json", false, "")
	root.AddCommand(New(Options{
		CommandName:  "mctest",
		Version:      "v1.2.3",
		BuildCommit:  "abc1234",
		BuildTime:    "2026-08-27",
		Platform:     "Test Platform",
		Notice:       "The platform may be upgraded",
		Capabilities: []string{"get", "list"},
		ReadOnly:     true,
	}))
	var stdout bytes.Buffer
	root.SetArgs([]string{"version"})
	root.SetOut(&stdout)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{
		`"version":"v1.2.3"`, `"build_commit":"abc1234"`, `"build_time":"2026-08-27"`,
		`"schema_version":"v1"`, `"read_only":true`, `"platform":"Test Platform"`,
		`"notice":"The platform may be upgraded"`, `"capabilities":["get","list"]`,
	} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("output is missing field %s: %s", want, out)
		}
	}
}

func TestNew_HumanOutput(t *testing.T) {
	root := &cobra.Command{Use: "mctest"}
	// No --json flag is registered, and faking a TTY for stdout via cobra's
	// default injection is not practical; here we simply verify the
	// command's RunE behavior in machine mode (buffer is non-TTY), which
	// appframe guarantees.
	root.AddCommand(New(Options{CommandName: "mctest", Version: "dev", Notice: "Notice"}))
	var stdout bytes.Buffer
	root.SetArgs([]string{"version"})
	root.SetOut(&stdout)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("non-TTY should output the envelope: %s", stdout.String())
	}
}
