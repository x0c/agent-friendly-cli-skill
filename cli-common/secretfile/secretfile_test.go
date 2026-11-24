package secretfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sampleConfig struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

// newTempDirWithEnv sets up a temporary config directory (tightened to 0700)
// and isolates it via a dedicated environment variable.
func newTempDirWithEnv(t *testing.T, envVar string) string {
	t.Helper()
	dir := t.TempDir()
	// t.TempDir may create 0755 depending on umask; tighten to 0700 to match
	// a real config directory.
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envVar, dir)
	return dir
}

func TestWriteReadRoundtrip(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	path := filepath.Join(dir, "config.json")
	in := sampleConfig{Name: "default", Secret: "credentials"}

	for _, policy := range []Policy{Strict, Tighten, OwnerOnly} {
		if err := Write(path, &in, policy); err != nil {
			t.Fatalf("policy %d: write failed: %v", policy, err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("policy %d: file not created: %v", policy, err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("policy %d: file permissions are %#o, want 0600", policy, info.Mode().Perm())
		}
		var out sampleConfig
		if err := Read(path, &out, policy); err != nil {
			t.Fatalf("policy %d: read failed: %v", policy, err)
		}
		if out != in {
			t.Fatalf("policy %d: round-trip content mismatch: %+v", policy, out)
		}
	}
}

func TestReadNotExistReturnsRawError(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	var out sampleConfig
	err := Read(filepath.Join(dir, "absent.json"), &out, Strict)
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("want not-exist error, got: %v", err)
	}
}

func TestReadRejectsSymlinkFile(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	real := filepath.Join(dir, "real.json")
	if err := os.WriteFile(real, []byte(`{"name":"x"}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not supported in this environment")
	}
	var out sampleConfig
	err := Read(link, &out, Strict)
	ue, ok := err.(*UnsafeError)
	if !ok || ue.Reason != "symlink" || ue.Kind != "file" {
		t.Fatalf("want file-symlink UnsafeError, got: %v", err)
	}
	if !strings.Contains(ue.Error(), "insecure config file") {
		t.Fatalf("error message inconsistent with existing implementation: %s", ue.Error())
	}
}

func TestReadRejectsWrongPerm(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"name":"x"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var out sampleConfig
	err := Read(path, &out, Strict)
	ue, ok := err.(*UnsafeError)
	if !ok || ue.Reason != "perm" || ue.Mode != 0644 {
		t.Fatalf("want permission UnsafeError(0644), got: %v", err)
	}
	if !strings.Contains(ue.Error(), "must be 0600") {
		t.Fatalf("error message inconsistent with existing implementation: %s", ue.Error())
	}
}

func TestOwnerOnlyAcceptsOwnerOnlyPerms(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	path := filepath.Join(dir, "config.json")
	// 0400: owner-readable, no group/other bits — OwnerOnly should accept it,
	// Strict should reject it.
	if err := os.WriteFile(path, []byte(`{"name":"x"}`), 0400); err != nil {
		t.Fatal(err)
	}
	var out sampleConfig
	if err := Read(path, &out, OwnerOnly); err != nil {
		t.Fatalf("OwnerOnly should accept 0400: %v", err)
	}
	if err := Read(path, &out, Strict); err == nil {
		t.Fatal("Strict should reject 0400")
	}
	// group-readable: OwnerOnly should reject it too.
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if err := Read(path, &out, OwnerOnly); err == nil {
		t.Fatal("OwnerOnly should reject 0640")
	}
}

func TestStrictDirRejectsLoosePerm(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	loose := filepath.Join(dir, "loose")
	if err := os.Mkdir(loose, 0755); err != nil {
		t.Fatal(err)
	}
	err := Write(filepath.Join(loose, "config.json"), &sampleConfig{}, Strict)
	ue, ok := err.(*UnsafeError)
	if !ok || ue.Kind != "dir" || ue.Reason != "perm" {
		t.Fatalf("want directory-permission UnsafeError, got: %v", err)
	}
}

func TestTightenDirAutoChmods(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	loose := filepath.Join(dir, "loose")
	if err := os.Mkdir(loose, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(loose, "config.json"), &sampleConfig{}, Tighten); err != nil {
		t.Fatalf("Tighten should auto-tighten instead of failing: %v", err)
	}
	info, err := os.Lstat(loose)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("directory should have been tightened to 0700, got %#o", info.Mode().Perm())
	}
}

func TestWriteRejectsSymlinkDir(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not supported in this environment")
	}
	for _, policy := range []Policy{Strict, Tighten, OwnerOnly} {
		err := Write(filepath.Join(link, "config.json"), &sampleConfig{}, policy)
		ue, ok := err.(*UnsafeError)
		if !ok || ue.Kind != "dir" || ue.Reason != "symlink" {
			t.Fatalf("policy %d: want directory-symlink UnsafeError, got: %v", policy, err)
		}
	}
}

func TestWriteReplaceExistingAtomic(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	path := filepath.Join(dir, "config.json")
	if err := Write(path, &sampleConfig{Name: "a"}, Strict); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, &sampleConfig{Name: "b", Secret: "new value"}, Strict); err != nil {
		t.Fatal(err)
	}
	var out sampleConfig
	if err := Read(path, &out, Strict); err != nil {
		t.Fatal(err)
	}
	if out.Name != "b" || out.Secret != "new value" {
		t.Fatalf("content mismatch after overwrite: %+v", out)
	}
	// Atomic writes must not leave temp files behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Fatalf("leftover temp file: %s", entry.Name())
		}
	}
}

func TestReadInvalidJSON(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	var out sampleConfig
	err := Read(path, &out, Strict)
	if err == nil || !strings.Contains(err.Error(), "failed to parse config file") {
		t.Fatalf("want parse error, got: %v", err)
	}
}

// TestJSONFieldCompatibility locks down the on-disk format: two-space
// indentation and field names identical to existing files.
func TestJSONFieldCompatibility(t *testing.T) {
	dir := newTempDirWithEnv(t, "TEST_SECRETFILE_HOME")
	path := filepath.Join(dir, "config.json")
	if err := Write(path, &sampleConfig{Name: "n", Secret: "s"}, Strict); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := "{\n  \"name\": \"n\",\n  \"secret\": \"s\"\n}"
	if string(data) != expected {
		t.Fatalf("on-disk format inconsistent with existing implementation:\n%s", string(data))
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
}
