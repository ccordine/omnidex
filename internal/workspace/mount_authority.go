package workspace

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type authoritativeWorkspaceRoot struct {
	*os.Root
	authorityFD int
	mountID     uint64
}

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

func (root *authoritativeWorkspaceRoot) Lstat(relative string) (os.FileInfo, error) {
	if root == nil || root.Root == nil {
		return nil, fmt.Errorf("inspect workspace path %q: root authority is unavailable", relative)
	}
	info, err := root.Root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if err := root.requirePathMount(relative); err != nil {
		return nil, err
	}
	return info, nil
}

func (root *authoritativeWorkspaceRoot) Open(relative string) (*os.File, error) {
	if root == nil || root.Root == nil {
		return nil, fmt.Errorf("open workspace path %q: root authority is unavailable", relative)
	}
	file, err := root.Root.Open(relative)
	if err != nil {
		return nil, err
	}
	mountID, mountErr := workspaceMountIDForHandle(file)
	if mountErr != nil || mountID != root.mountID {
		if mountErr == nil {
			mountErr = fmt.Errorf("workspace path %q crosses the authoritative root mount", relative)
		}
		return nil, errors.Join(mountErr, file.Close())
	}
	return file, nil
}

func (root *authoritativeWorkspaceRoot) OpenFile(
	relative string,
	flag int,
	mode os.FileMode,
) (*os.File, error) {
	if root == nil || root.Root == nil {
		return nil, fmt.Errorf("open workspace file %q: root authority is unavailable", relative)
	}
	file, err := root.Root.OpenFile(relative, flag, mode)
	if err != nil {
		return nil, err
	}
	mountID, mountErr := workspaceMountIDForHandle(file)
	if mountErr != nil || mountID != root.mountID {
		if mountErr == nil {
			mountErr = fmt.Errorf("workspace file %q crosses the authoritative root mount", relative)
		}
		return nil, errors.Join(mountErr, file.Close())
	}
	return file, nil
}
