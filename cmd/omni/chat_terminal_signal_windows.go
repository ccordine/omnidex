//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func enableTerminalInterruptSignal(fd int) error {
	handle := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return fmt.Errorf("read terminal interrupt settings: %w", err)
	}
	mode |= windows.ENABLE_PROCESSED_INPUT
	if err := windows.SetConsoleMode(handle, mode); err != nil {
		return fmt.Errorf("enable terminal interrupt signal: %w", err)
	}
	return nil
}
