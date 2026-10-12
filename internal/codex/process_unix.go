//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package codex

import (
	"os/exec"
	"syscall"
)

func configureProcessLifetime(process *exec.Cmd) {
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Cancel = func() error { return syscall.Kill(-process.Process.Pid, syscall.SIGKILL) }
}

func interruptProcess(process *exec.Cmd) error {
	return syscall.Kill(-process.Process.Pid, syscall.SIGINT)
}
