//go:build linux

package projectroot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/model"
	"golang.org/x/sys/unix"
)

// DirectoryIdentity reads the device and inode of a local directory. It is
// used to notice directory replacement, not to authenticate a host or archive
// filesystem contents. Filesystem export-handle support is not required.
func DirectoryIdentity(path string) (identity string, resultErr error) {
	if err := model.ValidateChannelWorkspaceRoot(path); err != nil {
		return "", err
	}
	if filepath.Clean(path) != path {
		return "", fmt.Errorf("directory identity requires one canonical absolute path")
	}
	expected, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect directory identity path %q: %w", path, err)
	}
	if !expected.IsDir() || expected.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("directory identity path %q is not one exact directory", path)
	}
	directory, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open directory identity path %q: %w", path, err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, directory.Close())
	}()
	opened, err := directory.Stat()
	if err != nil || !opened.IsDir() || !os.SameFile(expected, opened) {
		if err == nil {
			err = fmt.Errorf("opened directory differs from the exact requested path")
		}
		return "", fmt.Errorf("verify directory identity path %q: %w", path, err)
	}

	var status unix.Stat_t
	if err := unix.Fstat(int(directory.Fd()), &status); err != nil {
		return "", fmt.Errorf("stat directory identity path %q: %w", path, err)
	}
	identity = fmt.Sprintf("%s%d_%d", directoryIdentityPrefix, status.Dev, status.Ino)
	if err := ValidateDirectoryIdentity(identity); err != nil {
		return "", fmt.Errorf("invalid directory stat for %q: %w", path, err)
	}
	return identity, nil
}
