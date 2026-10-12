//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package codex

import (
	"os"
	"os/exec"
)

func configureProcessLifetime(process *exec.Cmd) {}

func interruptProcess(process *exec.Cmd) error { return process.Process.Signal(os.Interrupt) }
