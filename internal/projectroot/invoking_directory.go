package projectroot

import (
	"errors"
	"fmt"
	"os"
)

// OpenInvokingDirectory anchors the actual cwd before resolving its display
// path. Reopening that path later cannot substitute another directory.
func OpenInvokingDirectory() (*DirectoryHandle, error) {
	root, err := os.OpenRoot(".")
	if err != nil {
		return nil, fmt.Errorf("open invoking directory: %w", err)
	}
	directory, err := root.Open(".")
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	handle := &DirectoryHandle{root: root, directory: directory}
	handle.info, err = directory.Stat()
	if err != nil {
		return nil, errors.Join(err, handle.Close())
	}
	current, err := os.Getwd()
	if err != nil {
		return nil, errors.Join(err, handle.Close())
	}
	handle.path, err = ResolvePhysicalDirectory(current)
	if err != nil {
		return nil, errors.Join(err, handle.Close())
	}
	if _, err := handle.Identity(); err != nil {
		return nil, errors.Join(err, handle.Close())
	}
	return handle, nil
}
