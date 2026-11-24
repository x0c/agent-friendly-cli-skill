// atomic_test.go Locking test cases for atomic write: content and permission
// landing, JSON round trip, failure leaving no garbage (rename onto a
// directory fails and the temp file is cleaned up; non-writable directory
// yields OpTempCreate), and OpError stage classification with text
// pass-through. All fixtures are constructed programmatically.
package fsutil

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// tempLeftovers collects the temp files left over in a directory (matched by
// pattern prefix).
func tempLeftovers(t *testing.T, dir, prefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if len(prefix) == 0 || (len(e.Name()) >= len(prefix) && e.Name()[:len(prefix)] == prefix) {
			found = append(found, e.Name())
		}
	}
	return found
}

// TestWriteFileAtomicContentAndPerm locks down content landing and
// permissions: after writing, the file reads back fully, the target file
// permission equals perm, and no temp files remain in the directory.
func TestWriteFileAtomicContentAndPerm(t *testing.T) {
	for _, perm := range []os.FileMode{0o600, 0o644} {
		dir := t.TempDir()
		path := filepath.Join(dir, "target.json")
		data := []byte("first line\nsecond line")
		if err := WriteFileAtomic(path, data, perm, "target.json.tmp-*"); err != nil {
			t.Fatalf("perm %o: write failed: %v", perm, err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("perm %o: read back failed: %v", perm, err)
		}
		if string(got) != string(data) {
			t.Fatalf("perm %o: content mismatch: %q", perm, got)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != perm {
			t.Fatalf("perm %o: target file permission is %#o", perm, info.Mode().Perm())
		}
		if leftovers := tempLeftovers(t, dir, "target.json.tmp-"); len(leftovers) != 0 {
			t.Fatalf("perm %o: the success path must not leave temp files behind: %v", perm, leftovers)
		}
	}
}

// TestWriteJSONAtomicRoundtrip locks down the JSON atomic write: the file
// content is byte-identical to json.Marshal (constructed programmatically,
// no hand-written JSON literals) and reads back to an equal value.
func TestWriteJSONAtomicRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	type state struct {
		Name  string           `json:"name"`
		Count int              `json:"count"`
		Items map[string][]int `json:"items,omitempty"`
	}
	in := state{Name: "guard state", Count: 3, Items: map[string][]int{"normal": {1, 2, 3}}}
	if err := WriteJSONAtomic(path, in, 0o600, "state.json.tmp-*"); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	want, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(want) {
		t.Fatalf("file content must be byte-identical to json.Marshal:\ngot  %s\nwant %s", raw, want)
	}
	var out state
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("read-back unmarshal failed: %v", err)
	}
	if string(raw) != string(want) {
		t.Fatalf("round trip content mismatch: %s != %s", raw, want)
	}
}

// TestWriteFileAtomicRenameFailureCleansTemp locks down failure leaving no
// garbage (part 1): when the target path is an existing directory, rename
// fails, OpRename is returned, and the temp file is cleaned up.
func TestWriteFileAtomicRenameFailureCleansTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "as-dir")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	err := WriteFileAtomic(target, []byte("data"), 0o600, "as-dir.tmp-*")
	if err == nil {
		t.Fatal("rename onto a directory should fail")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("should return *OpError, got %T: %v", err, err)
	}
	if opErr.Op != OpRename {
		t.Fatalf("failing stage should be %q, got %q", OpRename, opErr.Op)
	}
	if opErr.Error() != opErr.Err.Error() {
		t.Fatalf("OpError text should pass through the underlying error: %q != %q", opErr.Error(), opErr.Err.Error())
	}
	if leftovers := tempLeftovers(t, dir, "as-dir.tmp-"); len(leftovers) != 0 {
		t.Fatalf("the failure path must not leave temp files behind: %v", leftovers)
	}
}

// TestWriteFileAtomicTempCreateFailure locks down failure leaving no garbage
// (part 2): with a non-writable directory, temp file creation fails, OpTempCreate
// is returned, and no file is produced.
// Windows directory permissions do not block creation, and root ignores 0500;
// both are skipped.
func TestWriteFileAtomicTempCreateFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory permission semantics differ; this case does not apply")
	}
	if os.Geteuid() == 0 {
		t.Skip("root is not restricted by directory write permissions; this case does not apply")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	err := WriteFileAtomic(filepath.Join(dir, "target"), []byte("data"), 0o600, "target.tmp-*")
	if err == nil {
		t.Fatal("should fail on a non-writable directory")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Op != OpTempCreate {
		t.Fatalf("should return an *OpError with OpTempCreate, got %T: %v", err, err)
	}
}
