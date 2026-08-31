//go:build darwin

package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func enableTerminalInterruptSignal(fd int) error {
	settings, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return fmt.Errorf("read terminal interrupt settings: %w", err)
	}
	settings.Lflag |= unix.ISIG
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, settings); err != nil {
		return fmt.Errorf("enable terminal interrupt signal: %w", err)
	}
	return nil
}
