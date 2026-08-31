package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func workspacePath(root, relative string) string {
	return filepath.Join(root, filepath.FromSlash(relative))
}

func inspectWorkspaceParents(
	root string,
	relative string,
	create bool,
) (created []string, missing bool, resultErr error) {
	current := root
	parts := strings.Split(relative, "/")
	for index, name := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(name))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if !create {
				return created, true, nil
			}
			if err := os.Mkdir(current, 0o755); err != nil {
				return created, false, fmt.Errorf("create workspace parent for %q: %w", relative, err)
			}
			created = append(created, strings.Join(parts[:index+1], "/"))
			info, err = os.Lstat(current)
		}
		if err != nil {
			return created, false, fmt.Errorf("inspect workspace parent for %q: %w", relative, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return created, false, fmt.Errorf("workspace path %q has a non-directory or symlink parent", relative)
		}
	}
	return created, false, nil
}

func regularFileMatches(
	ctx context.Context,
	absolute string,
	before os.FileInfo,
	content []byte,
	mode uint32,
) bool {
	if before == nil || before.Mode().Perm() != os.FileMode(mode) {
		return false
	}
	return regularFileContentMatches(ctx, absolute, before, content)
}

func regularFileContentMatches(
	ctx context.Context,
	absolute string,
	before os.FileInfo,
	content []byte,
) bool {
	if before == nil || !before.Mode().IsRegular() || before.Size() != int64(len(content)) {
		return false
	}
	if err := ctx.Err(); err != nil {
		return false
	}
	file, err := os.Open(absolute)
	if err != nil {
		return false
	}
	read, readErr := io.ReadAll(io.LimitReader(file, int64(len(content))+1))
	closeErr := file.Close()
	after, statErr := os.Lstat(absolute)
	if readErr != nil || closeErr != nil || statErr != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return false
	}
	return bytes.Equal(read, content)
}

func reserveSiblingPath(parent string, pattern string) (string, error) {
	placeholder, err := os.CreateTemp(parent, pattern)
	if err != nil {
		return "", err
	}
	name := placeholder.Name()
	closeErr := placeholder.Close()
	removeErr := os.Remove(name)
	if closeErr != nil || removeErr != nil {
		return "", errors.Join(closeErr, removeErr)
	}
	return name, nil
}

func writePreparedFile(
	ctx context.Context,
	parent string,
	content []byte,
	mode uint32,
) (_ string, _ os.FileInfo, resultErr error) {
	file, err := os.CreateTemp(parent, ".omnidex-write-")
	if err != nil {
		return "", nil, err
	}
	name := file.Name()
	keep := false
	defer func() {
		if file != nil {
			resultErr = errors.Join(resultErr, file.Close())
		}
		if !keep {
			resultErr = errors.Join(resultErr, removeIfPresent(name))
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
	if !info.Mode().IsRegular() || info.Size() != int64(len(content)) || info.Mode().Perm() != os.FileMode(mode) {
		return "", nil, fmt.Errorf("prepared workspace file differs from desired bytes or permissions")
	}
	if err := file.Close(); err != nil {
		file = nil
		return "", nil, err
	}
	file = nil
	keep = true
	return name, info, nil
}

func removeIfPresent(name string) error {
	err := os.Remove(name)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func removeCreatedDirectories(root string, created []string) error {
	var failures []error
	seen := make(map[string]struct{}, len(created))
	for index := len(created) - 1; index >= 0; index-- {
		relative := created[index]
		if _, duplicate := seen[relative]; duplicate {
			continue
		}
		seen[relative] = struct{}{}
		absolute := workspacePath(root, relative)
		err := os.Remove(absolute)
		if err != nil && !os.IsNotExist(err) {
			if !errors.Is(err, syscall.ENOTEMPTY) && !errors.Is(err, syscall.EEXIST) {
				failures = append(failures, fmt.Errorf("remove created workspace directory %q: %w", relative, err))
			}
		}
	}
	return errors.Join(failures...)
}
