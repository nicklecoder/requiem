//go:build !windows

package requiem

import (
	"os/exec"
	"syscall"
)

// detach starts the child in its own session, so it outlives the command
// that started it and a terminal closing does not signal it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
