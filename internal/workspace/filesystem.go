package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

func inspectWorkspaceParents(
	root *os.Root,
	relative string,
	create bool,
	result *ReconciliationResult,
) (bool, error) {
	if root == nil {
		return false, fmt.Errorf("workspace parent inspection requires an open root")
	}
	current := ""
	parts := strings.Split(relative, "/")
	for _, name := range parts[:len(parts)-1] {
		current = path.Join(current, name)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			if !create {
				return true, nil
			}
			if err := root.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
				return false, fmt.Errorf("create workspace parent for %q: %w", relative, err)
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return false, fmt.Errorf("inspect workspace parent for %q: %w", relative, err)
		}
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if !create {
			return true, nil
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return false, fmt.Errorf("workspace path %q has a special filesystem parent", relative)
		}
		if err := root.Remove(current); err != nil {
			return false, fmt.Errorf("replace workspace parent for %q: %w", relative, err)
		}
		if result != nil {
			result.Changes = append(result.Changes, Change{Path: current, Kind: ChangeDelete})
		}
		if err := root.Mkdir(current, 0o755); err != nil {
			return false, fmt.Errorf("create workspace parent for %q: %w", relative, err)
		}
	}
	return false, nil
}

func openRegularFileContentMatch(
	ctx context.Context,
	root *os.Root,
	relative string,
	before os.FileInfo,
	content []byte,
) (*os.File, bool, error) {
	if ctx == nil || root == nil || before == nil || !before.Mode().IsRegular() ||
		before.Size() != int64(len(content)) || ctx.Err() != nil {
		return nil, false, nil
	}
	file, err := root.Open(relative)
	if err != nil {
		if len(content) == 0 {
			after, statErr := root.Lstat(relative)
			if statErr != nil {
				return nil, false, fmt.Errorf("verify empty workspace file %q: %w", relative, statErr)
			}
			if after.Mode().IsRegular() && os.SameFile(before, after) && after.Size() == 0 &&
				before.Mode() == after.Mode() && before.ModTime().Equal(after.ModTime()) {
				return nil, true, nil
			}
		}
		return nil, false, nil
	}
	read, readErr := io.ReadAll(io.LimitReader(file, int64(len(content))+1))
	fileInfo, fileStatErr := file.Stat()
	pathInfo, pathStatErr := root.Lstat(relative)
	if readErr != nil || fileStatErr != nil || pathStatErr != nil ||
		!os.SameFile(before, fileInfo) || !os.SameFile(before, pathInfo) ||
		before.Mode() != pathInfo.Mode() || before.Size() != pathInfo.Size() ||
		!before.ModTime().Equal(pathInfo.ModTime()) || !bytes.Equal(read, content) {
		return file, false, nil
	}
	return file, true, nil
}

func createSiblingTemporary(
	root *os.Root,
	parent string,
) (*os.File, string, error) {
	if root == nil {
		return nil, "", fmt.Errorf("workspace temporary file requires an open root")
	}
	for attempt := 0; attempt < 64; attempt++ {
		var entropy [16]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return nil, "", err
		}
		name := ".omnidex-write-" + hex.EncodeToString(entropy[:])
		if parent != "." && parent != "" {
			name = path.Join(parent, name)
		}
		file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		return file, name, nil
	}
	return nil, "", fmt.Errorf("reserve unique workspace temporary file")
}

func writePreparedFile(
	ctx context.Context,
	root *os.Root,
	parent string,
	content []byte,
	mode uint32,
) (_ string, _ os.FileInfo, resultErr error) {
	file, name, err := createSiblingTemporary(root, parent)
	if err != nil {
		return "", nil, err
	}
	keep := false
	defer func() {
		if file != nil {
			resultErr = errors.Join(resultErr, file.Close())
		}
		if !keep {
			resultErr = errors.Join(resultErr, removeIfPresent(root, name))
		}
	}()
	written := 0
	for written < len(content) {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		count, err := file.Write(content[written:])
		if err != nil {
			return "", nil, err
		}
		if count == 0 {
			return "", nil, io.ErrShortWrite
		}
		written += count
	}
	if err := file.Sync(); err != nil {
		return "", nil, err
	}
	if err := file.Chmod(os.FileMode(mode)); err != nil {
		return "", nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(content)) ||
		info.Mode().Perm() != os.FileMode(mode) {
		return "", nil, fmt.Errorf("prepared workspace file differs from desired bytes or permissions")
	}
	if err := file.Close(); err != nil {
		return "", nil, fmt.Errorf("close prepared workspace file %q: %w", name, err)
	}
	file = nil
	keep = true
	return name, info, nil
}

func removeIfPresent(root *os.Root, name string) error {
	if root == nil {
		return fmt.Errorf("workspace removal requires an open root")
	}
	err := root.Remove(name)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}
