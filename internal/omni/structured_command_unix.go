//go:build !windows

package omni

import (
	"os/exec"
	"syscall"
)

func newStructuredShellCommand(command string) *exec.Cmd {
	return exec.Command("bash", "-o", "pipefail", "-c", command)
}

func configureStructuredCommandProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killStructuredCommandProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
