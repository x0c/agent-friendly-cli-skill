// Package secretfile provides unified secure reading and writing of files that
// hold sensitive information: symlinks and non-regular files are rejected,
// permissions are validated according to policy, and content is persisted via
// a 0600 temp file plus atomic replacement. Error messages match the existing
// CLI implementations, so user-visible behavior is unchanged after adoption.
package secretfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Policy controls how directory and file permissions are validated, matching
// the existing behavior of the individual CLIs.
type Policy int

const (
	// Strict requires the directory to be exactly 0700 and the file exactly
	// 0600; anything else is an error (the current behavior of legacy-a and
	// similar projects).
	Strict Policy = iota
	// Tighten requires the file to be exactly 0600; when directory permissions
	// are non-compliant they are automatically tightened to 0700 instead of
	// raising an error (the current behavior of legacy-b).
	Tighten
	// OwnerOnly rejects a file if anyone other than its owner can read or
	// write it (any group/other permission bit triggers rejection); for
	// directories only symlinks are rejected and the entry must be a
	// directory, permission bits are not checked (the current behavior of
	// legacy-c).
	OwnerOnly
)

// UnsafeError indicates that a file or directory failed the security check and
// the read or write is refused.
type UnsafeError struct {
	Path   string      // path that failed the check
	Kind   string      // "dir" or "file"
	Reason string      // "symlink" (symlink or non-regular file) or "perm" (non-compliant permissions)
	Mode   os.FileMode // actual permission bits, only meaningful when Reason is perm
}

// Error returns the error message kept consistent with each project's existing
// implementation.
func (e *UnsafeError) Error() string {
	if e.Reason == "perm" {
		if e.Kind == "dir" {
			return fmt.Sprintf(msgDirPermUnsafe, e.Path)
		}
		return fmt.Sprintf(msgFilePermUnsafe, e.Path)
	}
	if e.Kind == "dir" {
		return fmt.Sprintf(msgDirUnsafe, e.Path)
	}
	return fmt.Sprintf(msgFileUnsafe, e.Path)
}

// permCheckEnabled reports whether the current platform validates POSIX
// permission bits; Windows lacks that semantic, so permission checks are
// skipped there (symlink and regular-file checks still apply).
func permCheckEnabled() bool { return runtime.GOOS != "windows" }

func ensureDir(dir string, policy Policy) error {
	info, err := os.Lstat(dir)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return &UnsafeError{Path: dir, Kind: "dir", Reason: "symlink"}
		}
		if !permCheckEnabled() || policy == OwnerOnly {
			return nil
		}
		if info.Mode().Perm() != 0700 {
			if policy == Tighten {
				// Directory already exists: tighten it to 0700 (idempotent).
				// Do not fail on permissions of an externally pre-created
				// directory, but guarantee the final permissions are 0700 so
				// credentials never land in a loosely-permissioned directory.
				if err := os.Chmod(dir, 0700); err != nil {
					return fmt.Errorf(msgTightenDirFailed, dir, err)
				}
				return nil
			}
			return &UnsafeError{Path: dir, Kind: "dir", Reason: "perm", Mode: info.Mode().Perm()}
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.Chmod(dir, 0700)
}

func ensureFile(path string, policy Policy) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return &UnsafeError{Path: path, Kind: "file", Reason: "symlink"}
	}
	if !permCheckEnabled() {
		return nil
	}
	if policy == OwnerOnly {
		if info.Mode().Perm()&0o077 != 0 {
			return &UnsafeError{Path: path, Kind: "file", Reason: "perm", Mode: info.Mode().Perm()}
		}
		return nil
	}
	if info.Mode().Perm() != 0600 {
		return &UnsafeError{Path: path, Kind: "file", Reason: "perm", Mode: info.Mode().Perm()}
	}
	return nil
}

// Read safely loads a JSON file into target: the file passes the security
// check first, then it is read and parsed. A missing file returns the
// not-exist error as-is so the caller decides the semantics.
func Read(path string, target any, policy Policy) error {
	if err := ensureFile(path, policy); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf(msgParseConfigFailed, err)
	}
	return nil
}

// Write safely persists a JSON file: directory validation (or tightening),
// file validation, then a 0600 temp file + Sync + Rename atomic replacement,
// which avoids half-written files and keeps credentials from leaking into
// loosely-permissioned temp files.
func Write(path string, value any, policy Policy) error {
	dir := filepath.Dir(path)
	if err := ensureDir(dir, policy); err != nil {
		return err
	}
	if err := ensureFile(path, policy); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
