//go:build !linux

package omni

import (
	"errors"
	"time"
)

var errTerminalInterruptUnsupported = errors.New("terminal esc interrupt is not supported on this platform")

func enableTerminalCbreak(fd int) (func(), error) {
	return nil, errTerminalInterruptUnsupported
}

func readTerminalByte(fd int, buffer []byte) (int, error) {
	return 0, errTerminalInterruptUnsupported
}

func pollTerminalInput(fd int, timeout time.Duration) (bool, error) {
	return false, errTerminalInterruptUnsupported
}
