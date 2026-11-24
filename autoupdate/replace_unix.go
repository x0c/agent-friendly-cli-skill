//go:build !windows

package autoupdate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// replaceExecutable performs an atomic replacement of target with newPath.
func replaceExecutable(target, newPath string) error {
	dir := filepath.Dir(target)
	info, err := os.Stat(target)
	mode := os.FileMode(0o755)
	if err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".autoupdate-*")
	if err != nil {
		return fmt.Errorf(msgTempFileFailed, err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	src, err := os.Open(newPath)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	_, copyErr := io.Copy(tmp, src)
	_ = src.Close()
	if copyErr != nil {
		_ = tmp.Close()
		return copyErr
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	tmpName = ""
	return nil
}
