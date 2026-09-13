//go:build linux

package workspace

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func workspaceMountIDForHandle(file *os.File) (uint64, error) {
	if file == nil {
		return 0, fmt.Errorf("workspace mount authority handle is unavailable")
	}
	var stat unix.Statx_t
	if err := unix.Statx(
		int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_STATX_SYNC_AS_STAT,
		unix.STATX_MNT_ID, &stat,
	); err != nil {
		return 0, err
	}
	if stat.Mask&unix.STATX_MNT_ID == 0 {
		return 0, fmt.Errorf("filesystem did not return mount identity")
	}
	return stat.Mnt_id, nil
}

func (root *authoritativeWorkspaceRoot) requirePathMount(relative string) error {
	if root == nil || root.Root == nil || root.authorityFD < 0 || root.mountID == 0 {
		return fmt.Errorf("workspace path mount check requires exact root authority")
	}
	var stat unix.Statx_t
	if err := unix.Statx(
		root.authorityFD, relative,
		unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_SYNC_AS_STAT,
		unix.STATX_MNT_ID, &stat,
	); err != nil {
		return fmt.Errorf("resolve mount for workspace path %q: %w", relative, err)
	}
	if stat.Mask&unix.STATX_MNT_ID == 0 {
		return fmt.Errorf("filesystem did not return mount identity for workspace path %q", relative)
	}
	if stat.Mnt_id != root.mountID {
		return fmt.Errorf("workspace path %q crosses the authoritative root mount", relative)
	}
	return nil
}

func renameWorkspaceEntry(oldParent *os.File, oldName string, newParent *os.File, newName string, noReplace bool) error {
	for {
		var err error
		if noReplace {
			err = unix.Renameat2(int(oldParent.Fd()), oldName, int(newParent.Fd()), newName, unix.RENAME_NOREPLACE)
		} else {
			err = unix.Renameat(int(oldParent.Fd()), oldName, int(newParent.Fd()), newName)
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return err
	}
}
