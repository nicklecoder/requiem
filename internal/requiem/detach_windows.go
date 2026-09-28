//go:build windows

package requiem

import (
	"os/exec"
	"syscall"
)

// detach starts the child without a console and in its own process group,
// so it outlives the command that started it.
func detach(cmd *exec.Cmd) {
	const createNewProcessGroup, detachedProcess = 0x00000200, 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}
