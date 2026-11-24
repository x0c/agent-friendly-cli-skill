//go:build !windows

package autoupdate

import (
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
