//go:build darwin

package workspace

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func workspaceMountIDForHandle(file *os.File) (uint64, error) {
	if file == nil {
		return 0, fmt.Errorf("workspace mount authority handle is unavailable")
	}
	return darwinMountID(int(file.Fd()))
}

func darwinMountID(fd int) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Fstatfs(fd, &stat); err != nil {
		return 0, err
	}
	return uint64(uint32(stat.Fsid.Val[0]))<<32 | uint64(uint32(stat.Fsid.Val[1])), nil
}

func (root *authoritativeWorkspaceRoot) requirePathMount(relative string) (resultErr error) {
	if root == nil || root.Root == nil || root.authorityFD < 0 {
		return fmt.Errorf("workspace path mount check requires exact root authority")
	}
	if err := validateReadPath(relative); err != nil {
		return err
	}
	// O_SYMLINK opens the link itself. O_EVTONLY requests metadata/event
	// access, so observing a mount does not read the entry's contents.
	fd, err := unix.Openat(root.authorityFD, relative, unix.O_EVTONLY|unix.O_SYMLINK|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open workspace mount observation %q: %w", relative, err)
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(fd)) }()
	mount, err := darwinMountID(fd)
	if err != nil {
		return err
	}
	if mount != root.mountID {
		return fmt.Errorf("workspace path %q crosses the authoritative root mount", relative)
	}
	return nil
}

func renameWorkspaceEntry(oldParent *os.File, oldName string, newParent *os.File, newName string, noReplace bool) error {
	flags := uint32(0)
	if noReplace {
		flags = unix.RENAME_EXCL
	}
	for {
		err := unix.RenameatxNp(int(oldParent.Fd()), oldName, int(newParent.Fd()), newName, flags)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return err
	}
}
