// atomic.go Atomic write via same-directory temp file + rename: write a temp
// file in the same directory first, then commit by renaming it, so concurrent
// readers never observe a partially written file. The temp file name carries a
// random unique suffix (os.CreateTemp), preventing multiple processes or
// goroutines writing the same .tmp from overwriting each other.
package fsutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic atomically writes data to path: it creates a temp file in
// the directory of path using tempPattern (os.CreateTemp; the pattern must
// contain *, e.g. "guard.json.tmp-*") → Chmod to perm → write → close →
// commit via rename. Any failing stage returns the corresponding *OpError and
// the temp file is cleaned up (no garbage left behind); a missing or
// non-writable directory fails as OpTempCreate — directory creation and its
// permissions are the caller's responsibility (policies differ per call site,
// see the package comment).
//
// Behavior: CreateTemp creates the file as 0600; Chmod(perm) matches the
// existing implementations at each call site (a harmless re-set when perm is
// already 0600); rename is atomic within the same directory.
func WriteFileAtomic(path string, data []byte, perm os.FileMode, tempPattern string) error {
	f, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return &OpError{Op: OpTempCreate, Err: err}
	}
	tmp := f.Name()
	if err := f.Chmod(perm); err != nil {
		f.Close()
		os.Remove(tmp)
		return &OpError{Op: OpChmod, Err: err}
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return &OpError{Op: OpWrite, Err: err}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return &OpError{Op: OpClose, Err: err}
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return &OpError{Op: OpRename, Err: err}
	}
	return nil
}

// WriteJSONAtomic serializes v to JSON and atomically writes it to path
// (parameter semantics and failure policy identical to WriteFileAtomic);
// serialization failure returns a wrapped non-OpError error.
func WriteJSONAtomic(path string, v any, perm os.FileMode, tempPattern string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return WriteFileAtomic(path, data, perm, tempPattern)
}
