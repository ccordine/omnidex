//go:build linux

package omni

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func enableTerminalCbreak(fd int) (func(), error) {
	var original syscall.Termios
	if err := ioctlTermios(fd, syscall.TCGETS, &original); err != nil {
		return nil, err
	}
	next := original
	next.Lflag &^= syscall.ICANON | syscall.ECHO
	next.Cc[syscall.VMIN] = 1
	next.Cc[syscall.VTIME] = 0
	if err := ioctlTermios(fd, syscall.TCSETS, &next); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = ioctlTermios(fd, syscall.TCSETS, &original)
		})
	}, nil
}

func readTerminalByte(fd int, buffer []byte) (int, error) {
	return syscall.Read(fd, buffer)
}

func ioctlTermios(fd int, request uintptr, termios *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(unsafe.Pointer(termios)))
	if errno != 0 {
		return errno
	}
	return nil
}

func pollTerminalInput(fd int, timeout time.Duration) (bool, error) {
	var readFDs syscall.FdSet
	fdSet(fd, &readFDs)
	timeval := syscall.NsecToTimeval(timeout.Nanoseconds())
	n, err := syscall.Select(fd+1, &readFDs, nil, nil, &timeval)
	if err != nil {
		if err == syscall.EINTR {
			return false, nil
		}
		return false, err
	}
	if n <= 0 {
		return false, nil
	}
	return fdIsSet(fd, &readFDs), nil
}

func fdSet(fd int, set *syscall.FdSet) {
	index := fd / 64
	offset := uint(fd % 64)
	if index >= 0 && index < len(set.Bits) {
		set.Bits[index] |= 1 << offset
	}
}

func fdIsSet(fd int, set *syscall.FdSet) bool {
	index := fd / 64
	offset := uint(fd % 64)
	if index < 0 || index >= len(set.Bits) {
		return false
	}
	return set.Bits[index]&(1<<offset) != 0
}
