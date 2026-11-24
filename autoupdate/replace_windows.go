//go:build windows

package autoupdate

import (
	"fmt"
	"io"
	"os"
)

// replaceExecutable writes the complete .new first, then moves the old file to
// .old, and finally puts .new in place, minimizing the window with no exe
// present.
func replaceExecutable(target, newPath string) error {
	newStaging := target + ".new"
	oldBackup := target + ".old"
	_ = os.Remove(newStaging)
	_ = os.Remove(oldBackup)

	src, err := os.Open(newPath)
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(newStaging, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		_ = src.Close()
		return err
	}
	_, copyErr := io.Copy(dst, src)
	_ = src.Close()
	if copyErr != nil {
		_ = dst.Close()
		_ = os.Remove(newStaging)
		return copyErr
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		_ = os.Remove(newStaging)
		return err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(newStaging)
		return err
	}

	if err := os.Rename(target, oldBackup); err != nil {
		_ = os.Remove(newStaging)
		return fmt.Errorf(msgMoveOldExeFailed, err)
	}
	if err := os.Rename(newStaging, target); err != nil {
		_ = os.Rename(oldBackup, target)
		_ = os.Remove(newStaging)
		return err
	}
	_ = os.Remove(oldBackup)
	return nil
}
