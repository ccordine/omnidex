package changeapply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func stageSnapshot(ctx context.Context, snapshot repositoryfacts.Snapshot) (string, error) {
	if err := validateStageInventory(snapshot); err != nil {
		return "", err
	}
	workspace, err := os.MkdirTemp("", "omnidex-repository-change-*")
	if err != nil {
		return "", fmt.Errorf("create repository change staging workspace: %w", err)
	}
	if err := createStageDirectories(workspace, snapshot.Files); err != nil {
		return "", joinCleanupError(err, os.RemoveAll(workspace))
	}
	for _, file := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return "", joinCleanupError(fmt.Errorf("stage repository snapshot: %w", err), os.RemoveAll(workspace))
		}
		source := filepath.Join(snapshot.Root, filepath.FromSlash(file.Path))
		destination := filepath.Join(workspace, filepath.FromSlash(file.Path))
		if err := rejectSourceSymlinkParents(snapshot.Root, file.Path); err != nil {
			return "", joinCleanupError(err, os.RemoveAll(workspace))
		}
		switch file.Kind {
		case repositoryfacts.EntryRegular:
			err = copyExactRegularFile(ctx, source, destination, file)
		case repositoryfacts.EntrySymlink:
			err = copyExactSymlink(source, destination, file)
		default:
			err = fmt.Errorf("repository snapshot file %q has unsupported kind %q", file.Path, file.Kind)
		}
		if err != nil {
			return "", joinCleanupError(err, os.RemoveAll(workspace))
		}
	}
	return workspace, nil
}

func validateStageInventory(snapshot repositoryfacts.Snapshot) error {
	entries := make(map[string]repositoryfacts.EntryKind, len(snapshot.Files))
	for _, file := range snapshot.Files {
		if protectedRepositoryPath(file.Path) {
			return fmt.Errorf("repository snapshot contains protected path %q and cannot be staged", file.Path)
		}
		if file.Mode&^uint32(0o777) != 0 {
			return fmt.Errorf("repository snapshot file %q has unsupported mode %o", file.Path, file.Mode)
		}
		if file.Kind == repositoryfacts.EntrySymlink {
			if err := validateStagedSymlinkTarget(file); err != nil {
				return err
			}
		}
		entries[file.Path] = file.Kind
	}
	for _, file := range snapshot.Files {
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(file.Path))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			if kind, exists := entries[parent]; exists {
				return fmt.Errorf(
					"repository snapshot path %q has tracked %s ancestor %q and cannot be staged",
					file.Path, kind, parent,
				)
			}
		}
	}
	return nil
}

func validateStagedSymlinkTarget(file repositoryfacts.File) error {
	if filepath.IsAbs(file.LinkTarget) {
		return fmt.Errorf("repository symlink %q has an absolute target and cannot enter isolated staging", file.Path)
	}
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(
		filepath.Dir(filepath.FromSlash(file.Path)), file.LinkTarget,
	)))
	if resolved == ".." || strings.HasPrefix(resolved, "../") || protectedRepositoryPath(resolved) {
		return fmt.Errorf("repository symlink %q escapes or targets protected staging state", file.Path)
	}
	return nil
}

func createStageDirectories(workspace string, files []repositoryfacts.File) error {
	set := make(map[string]struct{})
	for _, file := range files {
		for parent := filepath.Dir(filepath.FromSlash(file.Path)); parent != "."; parent = filepath.Dir(parent) {
			set[parent] = struct{}{}
		}
	}
	directories := make([]string, 0, len(set))
	for directory := range set {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(left, right int) bool {
		leftDepth := strings.Count(directories[left], string(filepath.Separator))
		rightDepth := strings.Count(directories[right], string(filepath.Separator))
		if leftDepth == rightDepth {
			return directories[left] < directories[right]
		}
		return leftDepth < rightDepth
	})
	for _, directory := range directories {
		if err := os.Mkdir(filepath.Join(workspace, directory), 0o755); err != nil {
			return fmt.Errorf("create staged repository directory %q: %w", filepath.ToSlash(directory), err)
		}
	}
	return nil
}

func rejectSourceSymlinkParents(root, relative string) error {
	current := root
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect repository source parent %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("repository source %q has an unsupported symlink or non-directory parent", relative)
		}
	}
	return nil
}

func copyExactRegularFile(ctx context.Context, source, destination string, expected repositoryfacts.File) error {
	before, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect repository file %q: %w", expected.Path, err)
	}
	if !before.Mode().IsRegular() || before.Mode().Perm() != os.FileMode(expected.Mode) || before.Size() != expected.Size {
		return fmt.Errorf("repository file %q differs from its exact kind, mode, or size", expected.Path)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open repository file %q: %w", expected.Path, err)
	}
	defer input.Close()
	opened, err := input.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return fmt.Errorf("repository file %q changed while it was opened", expected.Path)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create staged repository file %q: %w", expected.Path, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, hash), &contextReader{ctx: ctx, reader: input})
	if copyErr == nil {
		copyErr = output.Chmod(os.FileMode(expected.Mode))
	}
	if copyErr == nil {
		copyErr = output.Sync()
	}
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("copy repository file %q into staging: %w", expected.Path, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close staged repository file %q: %w", expected.Path, closeErr)
	}
	after, err := os.Lstat(source)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() ||
		!before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		return fmt.Errorf("repository file %q changed while it was staged", expected.Path)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != expected.SHA256 {
		return fmt.Errorf("repository file %q hash is stale; refresh the snapshot", expected.Path)
	}
	return nil
}

func copyExactSymlink(source, destination string, expected repositoryfacts.File) error {
	before, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect repository symlink %q: %w", expected.Path, err)
	}
	if before.Mode()&os.ModeSymlink == 0 || before.Mode().Perm() != os.FileMode(expected.Mode) {
		return fmt.Errorf("repository symlink %q differs from its exact kind or mode", expected.Path)
	}
	target, err := os.Readlink(source)
	if err != nil {
		return fmt.Errorf("read repository symlink %q: %w", expected.Path, err)
	}
	after, err := os.Lstat(source)
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		return fmt.Errorf("repository symlink %q changed while it was staged", expected.Path)
	}
	if target != expected.LinkTarget || int64(len(target)) != expected.Size || digest([]byte("symlink\x00"+target)) != expected.SHA256 {
		return fmt.Errorf("repository symlink %q identity is stale; refresh the snapshot", expected.Path)
	}
	if err := os.Symlink(target, destination); err != nil {
		return fmt.Errorf("create staged repository symlink %q: %w", expected.Path, err)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
