package projectroot

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

// DirectoryHandle retains the actual invoking directory. Every observation
// checks the named root against the retained handle, including after a rename.
type DirectoryHandle struct {
	mu        sync.Mutex
	path      string
	root      *os.Root
	directory *os.File
	info      os.FileInfo
}

func OpenDirectory(root string) (*DirectoryHandle, error) {
	physical, err := ResolvePhysicalDirectory(root)
	if err != nil {
		return nil, err
	}
	if physical != root {
		return nil, fmt.Errorf("directory handle requires its exact physical path")
	}
	expected, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	directory, err := rootFS.Open(".")
	if err != nil {
		return nil, errors.Join(err, rootFS.Close())
	}
	handle := &DirectoryHandle{path: root, root: rootFS, directory: directory, info: expected}
	if _, err := handle.Identity(); err != nil {
		return nil, errors.Join(err, handle.Close())
	}
	return handle, nil
}

func (handle *DirectoryHandle) Identity() (string, error) {
	if handle == nil {
		return "", fmt.Errorf("directory handle is unavailable")
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if err := handle.requireCurrentLocked(); err != nil {
		return "", err
	}
	return directoryIdentityForHandle(handle.directory)
}

func (handle *DirectoryHandle) requireCurrentLocked() error {
	if handle.directory == nil || handle.root == nil || handle.info == nil {
		return fmt.Errorf("directory handle is closed")
	}
	current, currentErr := os.Lstat(handle.path)
	opened, openedErr := handle.directory.Stat()
	anchored, anchoredErr := handle.root.Stat(".")
	if currentErr != nil || openedErr != nil || anchoredErr != nil {
		return errors.Join(fmt.Errorf("observe invoking directory"), currentErr, openedErr, anchoredErr)
	}
	if !current.IsDir() || !opened.IsDir() || !anchored.IsDir() ||
		!os.SameFile(handle.info, current) || !os.SameFile(handle.info, opened) ||
		!os.SameFile(handle.info, anchored) {
		return fmt.Errorf("invoking directory changed while its handle was retained")
	}
	return nil
}

func (handle *DirectoryHandle) Close() error {
	if handle == nil {
		return fmt.Errorf("directory handle is unavailable")
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.directory == nil || handle.root == nil {
		return fmt.Errorf("directory handle is already closed")
	}
	err := errors.Join(handle.directory.Close(), handle.root.Close())
	handle.directory, handle.root, handle.info = nil, nil, nil
	return err
}

func DirectoryIdentity(root string) (identity string, resultErr error) {
	handle, err := OpenDirectory(root)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, handle.Close()) }()
	return handle.Identity()
}
