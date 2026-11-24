//go:build windows

package autoupdate

import (
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	const (
		createNewProcessGroup = 0x00000200
		detachProcess         = 0x00000008
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachProcess,
		HideWindow:    true,
	}
}
