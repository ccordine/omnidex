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
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	relative string,
	create bool,
	result *ReconciliationResult,
	observer VerifiedChangeObserver,
) (bool, error) {
	if ctx == nil || root == nil {
		return false, fmt.Errorf("workspace parent inspection requires an open root")
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("inspect workspace parents: %w", err)
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
			if err := ctx.Err(); err != nil {
				return false, fmt.Errorf("create workspace parent for %q: %w", relative, err)
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
		if err := ctx.Err(); err != nil {
			return false, fmt.Errorf("replace workspace parent for %q: %w", relative, err)
		}
		if err := root.Remove(current); err != nil {
			return false, fmt.Errorf("replace workspace parent for %q: %w", relative, err)
		}
		if result != nil {
			if err := verifyAndRecordWorkspaceDeletion(root, result, observer, current); err != nil {
				return false, fmt.Errorf("verify removed workspace parent %q: %w", current, err)
			}
		}
		if err := root.Mkdir(current, 0o755); err != nil {
			return false, fmt.Errorf("create workspace parent for %q: %w", relative, err)
		}
	}
	return false, nil
}

func removeWorkspaceEntry(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	relative string,
) (resultErr error) {
	if ctx == nil || root == nil {
		return fmt.Errorf("workspace entry removal requires an open root")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("remove workspace entry %q: %w", relative, err)
	}
	info, err := root.Lstat(relative)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect workspace entry %q for removal: %w", relative, err)
	}
	if !info.IsDir() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("remove workspace entry %q: %w", relative, err)
		}
		if err := root.Remove(relative); err != nil {
			return fmt.Errorf("remove workspace entry %q: %w", relative, err)
		}
		return nil
	}
	directory, err := root.Open(relative)
	if err != nil {
		return fmt.Errorf("open workspace directory %q for removal: %w", relative, err)
	}
	defer func() {
		if directory != nil {
			resultErr = errors.Join(resultErr, directory.Close())
		}
	}()
	opened, err := directory.Stat()
	if err != nil || !opened.IsDir() || !os.SameFile(info, opened) {
		return fmt.Errorf("workspace directory %q changed before removal", relative)
	}
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("remove workspace directory %q: %w", relative, err)
		}
		entries, readErr := directory.ReadDir(128)
		for _, entry := range entries {
			if err := removeWorkspaceEntry(ctx, root, path.Join(relative, entry.Name())); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read workspace directory %q for removal: %w", relative, readErr)
		}
	}
	if err := directory.Close(); err != nil {
		directory = nil
		return fmt.Errorf("close workspace directory %q before removal: %w", relative, err)
	}
	directory = nil
	current, err := root.Lstat(relative)
	if err != nil || !current.IsDir() || !os.SameFile(info, current) {
		return fmt.Errorf("workspace directory %q changed during removal", relative)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("remove workspace directory %q: %w", relative, err)
	}
	if err := root.Remove(relative); err != nil {
		return fmt.Errorf("remove workspace directory %q: %w", relative, err)
	}
	return nil
}

func openRegularFileContentMatch(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	relative string,
	before os.FileInfo,
	content []byte,
) (*os.File, bool, error) {
	if ctx == nil || root == nil || before == nil {
		return nil, false, fmt.Errorf("workspace content comparison requires exact file authority")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, fmt.Errorf("compare workspace file %q: %w", relative, err)
	}
	if !before.Mode().IsRegular() || before.Size() != int64(len(content)) {
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
	if err := ctx.Err(); err != nil {
		return nil, false, errors.Join(
			fmt.Errorf("compare workspace file %q: %w", relative, err),
			closeWorkspaceHandle(relative, file),
		)
	}
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
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	parent string,
) (*os.File, string, error) {
	if ctx == nil || root == nil {
		return nil, "", fmt.Errorf("workspace temporary file requires an open root")
	}
	for attempt := 0; attempt < 64; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, "", fmt.Errorf("create workspace temporary file: %w", err)
		}
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
	root *authoritativeWorkspaceRoot,
	parent string,
	content []byte,
	mode uint32,
) (_ string, _ os.FileInfo, resultErr error) {
	file, name, err := createSiblingTemporary(ctx, root, parent)
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
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if err := file.Sync(); err != nil {
		return "", nil, err
	}
	if err := ctx.Err(); err != nil {
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

func removeIfPresent(root *authoritativeWorkspaceRoot, name string) error {
	if root == nil {
		return fmt.Errorf("workspace removal requires an open root")
	}
	err := root.Remove(name)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}
