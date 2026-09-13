package projectroot

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/workspace"
)

func (handle *DirectoryHandle) Path() string { return handle.path }

func (handle *DirectoryHandle) Stat(ctx context.Context, relative string) (workspace.Entry, error) {
	return readDirectoryHandle(handle, func(root *os.Root) (workspace.Entry, error) {
		return workspace.ReadEntry(ctx, root, relative)
	})
}

func (handle *DirectoryHandle) ReadFile(ctx context.Context, relative string) (workspace.File, error) {
	return readDirectoryHandle(handle, func(root *os.Root) (workspace.File, error) {
		return workspace.ReadFile(ctx, root, relative)
	})
}

func (handle *DirectoryHandle) ReadDirectory(ctx context.Context, relative, after string, limit int) (workspace.DirectoryPage, error) {
	return readDirectoryHandle(handle, func(root *os.Root) (workspace.DirectoryPage, error) {
		return workspace.ReadDirectory(ctx, root, relative, after, limit)
	})
}

func readDirectoryHandle[T any](handle *DirectoryHandle, read func(*os.Root) (T, error)) (T, error) {
	var zero T
	if handle == nil {
		return zero, fmt.Errorf("directory read requires retained authority")
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if err := handle.requireCurrentLocked(); err != nil {
		return zero, err
	}
	value, err := read(handle.root)
	return value, errors.Join(err, handle.requireCurrentLocked())
}
