//go:build linux || darwin

package workspace

import (
	"errors"
	"os"
	"syscall"
)

type nativeDirectoryLock struct{ directory *os.File }

func tryLockWorkspaceDirectory(directory *os.File) (directoryLock, error) {
	for {
		err := syscall.Flock(int(directory.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrWorkspaceBusy
		}
		if err != nil {
			return nil, err
		}
		return &nativeDirectoryLock{directory: directory}, nil
	}
}

func (lock *nativeDirectoryLock) Release() error {
	return syscall.Flock(int(lock.directory.Fd()), syscall.LOCK_UN)
}
