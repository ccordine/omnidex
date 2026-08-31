package workspace

import (
	"errors"
	"fmt"
	"os"
	"path"

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

func (root *authoritativeWorkspaceRoot) Rename(oldRelative, newRelative string) (resultErr error) {
	return root.rename(oldRelative, newRelative, false)
}

func (root *authoritativeWorkspaceRoot) RenameNoReplace(
	oldRelative string,
	newRelative string,
) (resultErr error) {
	return root.rename(oldRelative, newRelative, true)
}

func (root *authoritativeWorkspaceRoot) rename(
	oldRelative string,
	newRelative string,
	noReplace bool,
) (resultErr error) {
	if root == nil || root.Root == nil || root.authorityFD < 0 || root.mountID == 0 {
		return fmt.Errorf("rename workspace path: root authority is unavailable")
	}
	if err := validateRelativePath(oldRelative); err != nil {
		return fmt.Errorf("rename workspace source: %w", err)
	}
	if err := validateRelativePath(newRelative); err != nil {
		return fmt.Errorf("rename workspace destination: %w", err)
	}
	if _, err := root.Lstat(oldRelative); err != nil {
		return fmt.Errorf("inspect workspace rename source %q: %w", oldRelative, err)
	}
	if _, err := root.Lstat(newRelative); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect workspace rename destination %q: %w", newRelative, err)
	}

	oldParent, oldName, err := root.openRenameParent(oldRelative)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, oldParent.Close())
	}()
	newParent, newName, err := root.openRenameParent(newRelative)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, newParent.Close())
	}()

	for {
		if noReplace {
			err = unix.Renameat2(
				int(oldParent.Fd()), oldName,
				int(newParent.Fd()), newName,
				unix.RENAME_NOREPLACE,
			)
		} else {
			err = unix.Renameat(int(oldParent.Fd()), oldName, int(newParent.Fd()), newName)
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		break
	}
	if err != nil {
		return fmt.Errorf("rename workspace path %q to %q: %w", oldRelative, newRelative, err)
	}
	if err := root.requirePathMount(newRelative); err != nil {
		return fmt.Errorf("verify renamed workspace path %q: %w", newRelative, err)
	}
	return nil
}

func (root *authoritativeWorkspaceRoot) openRenameParent(relative string) (*os.File, string, error) {
	parentPath := path.Dir(relative)
	parent, err := root.Open(parentPath)
	if err != nil {
		return nil, "", fmt.Errorf("open workspace rename parent %q: %w", parentPath, err)
	}
	info, err := parent.Stat()
	if err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("workspace rename parent %q is not a directory", parentPath)
		}
		return nil, "", errors.Join(err, parent.Close())
	}
	return parent, path.Base(relative), nil
}
