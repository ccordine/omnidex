//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func workspaceMountIDForHandle(file *os.File) (uint64, error) {
	if file == nil {
		return 0, fmt.Errorf("workspace volume authority handle is unavailable")
	}
	identity, err := observeWindowsFileIdentity(windows.Handle(file.Fd()))
	return identity.VolumeSerialNumber, err
}

func (root *authoritativeWorkspaceRoot) requirePathMount(relative string) (resultErr error) {
	if root == nil || root.Root == nil || root.authorityFD < 0 {
		return fmt.Errorf("workspace volume check requires exact root authority")
	}
	if err := validateWindowsWorkspacePath(relative); err != nil {
		return err
	}
	handle := windows.Handle(root.authorityFD)
	if relative != "." {
		var err error
		handle, err = openWindowsWorkspaceEntry(handle, relative, windows.FILE_READ_ATTRIBUTES)
		if err != nil {
			return fmt.Errorf("open workspace volume observation %q: %w", relative, err)
		}
		defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(handle)) }()
	}
	identity, err := observeWindowsFileIdentity(handle)
	if err != nil {
		return err
	}
	if identity.VolumeSerialNumber != root.mountID {
		return fmt.Errorf("workspace path %q crosses the authoritative root volume", relative)
	}
	return nil
}
