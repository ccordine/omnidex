//go:build windows

package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsFileIdentity struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

func observeWindowsFileIdentity(handle windows.Handle) (windowsFileIdentity, error) {
	var identity windowsFileIdentity
	err := windows.GetFileInformationByHandleEx(handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&identity)), uint32(unsafe.Sizeof(identity)))
	return identity, err
}

func validateWindowsWorkspacePath(relative string) error {
	if err := validateReadPath(relative); err != nil {
		return err
	}
	if !filepath.IsLocal(filepath.FromSlash(relative)) {
		return fmt.Errorf("workspace path %q is not a local Windows name", relative)
	}
	if relative == "." {
		return nil
	}
	for _, name := range strings.Split(relative, "/") {
		if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
			return fmt.Errorf("workspace path %q contains a Windows name alias", relative)
		}
	}
	return nil
}

func openWindowsWorkspaceEntry(parent windows.Handle, relative string, access uint32) (windows.Handle, error) {
	if err := validateWindowsWorkspacePath(relative); err != nil {
		return 0, err
	}
	name, err := windows.NewNTUnicodeString(strings.ReplaceAll(relative, "/", "\\"))
	if err != nil {
		return 0, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, access|windows.SYNCHRONIZE, &attributes, &status, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return 0, windowsNativeError(err)
	}
	return handle, nil
}

func windowsNativeError(err error) error {
	if status, ok := err.(windows.NTStatus); ok {
		return status.Errno()
	}
	return err
}
