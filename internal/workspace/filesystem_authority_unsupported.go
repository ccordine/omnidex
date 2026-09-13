//go:build !linux && !darwin && !windows

package workspace

import (
	"fmt"
	"os"
	"runtime"
)

func unsupportedFilesystemAuthority() error {
	return fmt.Errorf("workspace mutation requires an implemented native filesystem authority adapter for %s", runtime.GOOS)
}

func workspaceMountIDForHandle(*os.File) (uint64, error) { return 0, unsupportedFilesystemAuthority() }
func (*authoritativeWorkspaceRoot) requirePathMount(string) error {
	return unsupportedFilesystemAuthority()
}
func renameWorkspaceEntry(*os.File, string, *os.File, string, bool) error {
	return unsupportedFilesystemAuthority()
}
func tryLockWorkspaceDirectory(*os.File) (directoryLock, error) {
	return nil, unsupportedFilesystemAuthority()
}
