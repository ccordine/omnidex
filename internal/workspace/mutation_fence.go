package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

const mutationFencePollInterval = 100 * time.Millisecond

var ErrWorkspaceBusy = errors.New("workspace mutation authority is already held")

// MutationFence is the cross-process authority to mutate one exact workspace
// directory. The operating system releases the advisory lock when the owning
// process or directory handle ends.
type MutationFence struct {
	mu        sync.Mutex
	root      string
	rootFS    *os.Root
	directory *os.File
	lock      directoryLock
	rootInfo  os.FileInfo
	mountID   uint64
	released  bool
}

func AcquireMutationFence(ctx context.Context, root string) (*MutationFence, error) {
	return acquireMutationFence(ctx, root, true)
}

// TryAcquireMutationFence distinguishes a held operating-system lock from an
// invalid root or unavailable platform. Remote callers can wait between RPCs.
func TryAcquireMutationFence(ctx context.Context, root string) (*MutationFence, error) {
	return acquireMutationFence(ctx, root, false)
}

func acquireMutationFence(ctx context.Context, root string, wait bool) (*MutationFence, error) {
	if ctx == nil {
		return nil, fmt.Errorf("acquire workspace mutation fence: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("acquire workspace mutation fence: %w", err)
	}
	expected, err := exactRootDirectory(root)
	if err != nil {
		return nil, err
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open authoritative workspace root %q: %w", root, err)
	}
	directory, err := rootFS.Open(".")
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("open workspace root mutation authority %q: %w", root, err),
			rootFS.Close(),
		)
	}
	opened, err := directory.Stat()
	if err != nil || !opened.IsDir() || !os.SameFile(expected, opened) {
		return nil, errors.Join(
			fmt.Errorf("workspace root %q changed while acquiring mutation authority", root),
			directory.Close(), rootFS.Close(),
		)
	}
	var lock directoryLock
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(
				fmt.Errorf("acquire workspace mutation fence for %q: %w", root, err),
				directory.Close(), rootFS.Close(),
			)
		}
		lock, err = tryLockWorkspaceDirectory(directory)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrWorkspaceBusy) {
			return nil, errors.Join(
				fmt.Errorf("lock workspace root mutation authority %q: %w", root, err),
				directory.Close(), rootFS.Close(),
			)
		}
		if !wait {
			return nil, errors.Join(ErrWorkspaceBusy, directory.Close(), rootFS.Close())
		}
		timer := time.NewTimer(mutationFencePollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, errors.Join(
				fmt.Errorf("acquire workspace mutation fence for %q: %w", root, ctx.Err()),
				directory.Close(), rootFS.Close(),
			)
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("acquire workspace mutation fence for %q: %w", root, err),
			releaseMutationFenceHandles(directory, rootFS, lock),
		)
	}
	current, err := os.Lstat(root)
	anchored, anchoredErr := rootFS.Stat(".")
	if err != nil || anchoredErr != nil || !current.IsDir() ||
		!os.SameFile(opened, current) || !os.SameFile(opened, anchored) {
		return nil, errors.Join(
			fmt.Errorf("workspace root %q changed after acquiring mutation authority", root),
			releaseMutationFenceHandles(directory, rootFS, lock),
		)
	}
	mountID, err := workspaceMountIDForHandle(directory)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("resolve workspace root mount authority %q: %w", root, err),
			releaseMutationFenceHandles(directory, rootFS, lock),
		)
	}
	return &MutationFence{
		root: root, rootFS: rootFS, directory: directory, lock: lock,
		rootInfo: opened, mountID: mountID,
	}, nil
}

func (fence *MutationFence) Release() error {
	if fence == nil {
		return fmt.Errorf("release workspace mutation fence: fence is unavailable")
	}
	fence.mu.Lock()
	defer fence.mu.Unlock()
	if fence.released || fence.directory == nil || fence.rootFS == nil {
		return fmt.Errorf("release workspace mutation fence for %q: fence was already released", fence.root)
	}
	directory := fence.directory
	rootFS := fence.rootFS
	lock := fence.lock
	fence.directory = nil
	fence.rootFS = nil
	fence.lock = nil
	fence.released = true
	if err := releaseMutationFenceHandles(directory, rootFS, lock); err != nil {
		return fmt.Errorf("release workspace mutation fence for %q: %w", fence.root, err)
	}
	return nil
}

func (fence *MutationFence) authoritativeRootLocked() (*authoritativeWorkspaceRoot, error) {
	if fence == nil || fence.released || fence.directory == nil || fence.rootFS == nil ||
		fence.rootInfo == nil || fence.lock == nil {
		return nil, fmt.Errorf("workspace mutation authority is unavailable")
	}
	locked, lockedErr := fence.directory.Stat()
	anchored, anchoredErr := fence.rootFS.Stat(".")
	current, currentErr := os.Lstat(fence.root)
	if lockedErr != nil || anchoredErr != nil || currentErr != nil ||
		!locked.IsDir() || !anchored.IsDir() || !current.IsDir() ||
		!os.SameFile(fence.rootInfo, locked) ||
		!os.SameFile(fence.rootInfo, anchored) ||
		!os.SameFile(fence.rootInfo, current) {
		return nil, fmt.Errorf("authoritative workspace root %q changed while fenced", fence.root)
	}
	mountID, err := workspaceMountIDForHandle(fence.directory)
	if err != nil {
		return nil, fmt.Errorf("resolve authoritative workspace root mount %q: %w", fence.root, err)
	}
	if mountID != fence.mountID {
		return nil, fmt.Errorf("authoritative workspace root mount %q changed while fenced", fence.root)
	}
	return &authoritativeWorkspaceRoot{
		Root: fence.rootFS, authorityFD: int(fence.directory.Fd()), mountID: fence.mountID,
	}, nil
}

func releaseMutationFenceHandles(directory *os.File, root *os.Root, lock directoryLock) error {
	var unlockErr error
	var directoryCloseErr error
	var rootCloseErr error
	if root == nil {
		rootCloseErr = fmt.Errorf("authoritative workspace root is unavailable")
	} else {
		rootCloseErr = root.Close()
	}
	if lock == nil {
		unlockErr = fmt.Errorf("workspace directory lock is unavailable")
	} else {
		unlockErr = lock.Release()
	}
	if directory == nil {
		directoryCloseErr = fmt.Errorf("workspace mutation fence directory is unavailable")
	} else {
		directoryCloseErr = directory.Close()
	}
	return errors.Join(unlockErr, directoryCloseErr, rootCloseErr)
}
