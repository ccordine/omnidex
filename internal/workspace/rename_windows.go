//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsRenameInformation struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [256]uint16
}

func renameWorkspaceEntry(oldParent *os.File, oldName string, newParent *os.File, newName string, noReplace bool) (resultErr error) {
	if err := validateWindowsWorkspacePath(newName); err != nil {
		return err
	}
	source, err := openWindowsWorkspaceEntry(windows.Handle(oldParent.Fd()), oldName, windows.DELETE|windows.FILE_READ_ATTRIBUTES)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(source)) }()
	name, err := windows.UTF16FromString(newName)
	if err != nil {
		return err
	}
	info := windowsRenameInformation{RootDirectory: windows.Handle(newParent.Fd())}
	if len(name) > len(info.FileName) {
		return fmt.Errorf("workspace rename basename exceeds its Windows UTF-16 bound")
	}
	copy(info.FileName[:], name)
	info.FileNameLength = uint32((len(name) - 1) * 2)
	if !noReplace {
		info.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS | windows.FILE_RENAME_IGNORE_READONLY_ATTRIBUTE
	}
	const fileRenameInformationEx = 65
	var status windows.IO_STATUS_BLOCK
	size := uint32(unsafe.Sizeof(info))
	err = windows.NtSetInformationFile(source, &status, (*byte)(unsafe.Pointer(&info)), size, fileRenameInformationEx)
	return windowsNativeError(err)
}
