//go:build linux

package projectroot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/model"
	"golang.org/x/sys/unix"
)

// DirectoryIdentity returns a kernel-backed identity for one exact physical
// directory. A same-host bind mount preserves the underlying filesystem ID,
// device/inode, and export handle; an equal path on another host does not
// establish this identity.
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
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(int(directory.Fd()), &filesystem); err != nil {
		return "", fmt.Errorf("stat filesystem identity for %q: %w", path, err)
	}
	handle, _, err := unix.NameToHandleAt(int(directory.Fd()), ".", 0)
	if err != nil {
		return "", fmt.Errorf("resolve export identity for %q: %w", path, err)
	}
	if handle.Size() < 1 || handle.Size() > 4096 {
		return "", fmt.Errorf("directory export identity for %q has invalid size %d", path, handle.Size())
	}

	digest := sha256.New()
	_, _ = digest.Write([]byte("omnidex.directory-identity.v1\x00"))
	for _, value := range []any{
		uint64(status.Dev), uint64(status.Ino), int64(filesystem.Type),
		filesystem.Fsid.Val[0], filesystem.Fsid.Val[1], handle.Type(), uint32(handle.Size()),
	} {
		if err := binary.Write(digest, binary.BigEndian, value); err != nil {
			return "", fmt.Errorf("encode directory identity for %q: %w", path, err)
		}
	}
	_, _ = digest.Write(handle.Bytes())
	return directoryIdentityPrefix + hex.EncodeToString(digest.Sum(nil)), nil
}
