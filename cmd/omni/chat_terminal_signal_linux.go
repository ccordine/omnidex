//go:build linux

package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func enableTerminalInterruptSignal(fd int) error {
	settings, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return fmt.Errorf("read terminal interrupt settings: %w", err)
	}
	settings.Lflag |= unix.ISIG
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, settings); err != nil {
		return fmt.Errorf("enable terminal interrupt signal: %w", err)
	}
	return nil
}
