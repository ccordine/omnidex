//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

type nativeDirectoryLock struct {
	release  chan struct{}
	finished chan error
	released atomic.Bool
}

func tryLockWorkspaceDirectory(directory *os.File) (directoryLock, error) {
	identity, err := observeWindowsFileIdentity(windows.Handle(directory.Fd()))
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(fmt.Sprintf("Global\\Omnidex.Workspace.%016x.%x", identity.VolumeSerialNumber, identity.FileID))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutexEx(nil, name, 0, windows.SYNCHRONIZE|windows.MUTEX_MODIFY_STATE)
	if handle == 0 {
		return nil, fmt.Errorf("open directory mutex: %w", err)
	}
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, errors.Join(err, windows.CloseHandle(handle))
	}
	lock := &nativeDirectoryLock{release: make(chan struct{}), finished: make(chan error, 1)}
	acquired := make(chan error, 1)
	go lock.hold(handle, acquired)
	if err := <-acquired; err != nil {
		return nil, err
	}
	return lock, nil
}

func (lock *nativeDirectoryLock) hold(handle windows.Handle, acquired chan<- error) {
	// Windows mutex ownership belongs to a thread, so acquisition and release
	// stay on this same OS thread for the complete lock lifetime.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		acquired <- errors.Join(err, windows.CloseHandle(handle))
		return
	}
	switch state {
	case windows.WAIT_OBJECT_0, windows.WAIT_ABANDONED:
		// This grants exclusion only. MutationFence reattests the directory,
		// and reconciliation independently checks expected file bytes.
	case uint32(windows.WAIT_TIMEOUT):
		acquired <- errors.Join(ErrWorkspaceBusy, windows.CloseHandle(handle))
		return
	default:
		acquired <- errors.Join(fmt.Errorf("directory mutex returned unexpected wait state %d", state), windows.CloseHandle(handle))
		return
	}
	acquired <- nil
	<-lock.release
	lock.finished <- errors.Join(windows.ReleaseMutex(handle), windows.CloseHandle(handle))
}

func (lock *nativeDirectoryLock) Release() error {
	if !lock.released.CompareAndSwap(false, true) {
		return fmt.Errorf("directory mutex was already released")
	}
	close(lock.release)
	return <-lock.finished
}
