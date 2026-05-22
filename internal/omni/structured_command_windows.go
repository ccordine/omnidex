//go:build windows

package omni

import (
	"os/exec"
	"syscall"
)

func newStructuredShellCommand(command string) *exec.Cmd {
	if _, err := exec.LookPath("pwsh.exe"); err == nil {
		return exec.Command("pwsh.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
	}
	if _, err := exec.LookPath("powershell.exe"); err == nil {
		return exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
	}
	return exec.Command("cmd.exe", "/C", command)
}

func configureStructuredCommandProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func killStructuredCommandProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
