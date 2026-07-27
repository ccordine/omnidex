package omni

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type patchMutation struct {
	path          string
	target        string
	action        string
	next          []byte
	original      []byte
	mode          os.FileMode
	staged        string
	backup        string
	originalMoved bool
	committed     bool
}

func preparePatchMutations(ctx context.Context, workspace string, files []parsedPatchFile, dryRun bool) ([]patchMutation, PatchApplyResult, error) {
	result := PatchApplyResult{Workspace: workspace, DryRun: dryRun}
	mutations := make([]patchMutation, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, result, fmt.Errorf("prepare patch: %w", err)
		}
		path := file.targetPath()
		if path == "" {
			return nil, result, fmt.Errorf("patch target is empty")
		}
		target, err := safeWorkspacePath(workspace, path)
		if err != nil {
			return nil, result, err
		}
		if _, duplicate := seen[target]; duplicate {
			return nil, result, fmt.Errorf("patch contains duplicate target %s", path)
		}
		seen[target] = struct{}{}

		action := file.action()
		mutation := patchMutation{path: path, target: target, action: action, mode: 0o644}
		if action == "create" {
			if _, err := os.Lstat(target); err == nil {
				return nil, result, fmt.Errorf("create patch target already exists: %s", path)
			} else if !os.IsNotExist(err) {
				return nil, result, fmt.Errorf("inspect create patch target %s: %w", path, err)
			}
		} else {
			info, err := os.Lstat(target)
			if err != nil {
				return nil, result, fmt.Errorf("inspect patch target %s: %w", path, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, result, fmt.Errorf("patch target %s is a symlink", path)
			}
			if !info.Mode().IsRegular() {
				return nil, result, fmt.Errorf("patch target %s is not a regular file", path)
			}
			mutation.mode = info.Mode().Perm()
			mutation.original, err = os.ReadFile(target)
			if err != nil {
				return nil, result, fmt.Errorf("read patch target %s: %w", path, err)
			}
		}
		next, err := applyParsedPatchContent(mutation.original, file)
		if err != nil {
			return nil, result, err
		}
		mutation.next = []byte(next)
		if action == "update" && bytes.Equal(mutation.original, mutation.next) {
			return nil, result, fmt.Errorf("patch update does not change target %s", path)
		}
		mutations = append(mutations, mutation)
		result.Files = append(result.Files, PatchFileResult{Path: path, Action: action})
	}
	return mutations, result, nil
}

func commitPatchMutations(ctx context.Context, workspace string, mutations []patchMutation) (err error) {
	createdDirs, err := createPatchParentDirectories(workspace, mutations)
	if err != nil {
		return err
	}
	rollbackRequired := true
	defer func() {
		if err != nil && rollbackRequired {
			err = errors.Join(err, rollbackPatchMutations(mutations), removePatchDirectories(createdDirs))
		}
	}()

	if err = stagePatchMutations(ctx, mutations); err != nil {
		return err
	}
	for index := range mutations {
		mutation := &mutations[index]
		if err = ctx.Err(); err != nil {
			return fmt.Errorf("commit patch: %w", err)
		}
		if err = verifyPatchTargetUnchanged(*mutation); err != nil {
			return err
		}
		switch mutation.action {
		case "create":
			if err = os.Rename(mutation.staged, mutation.target); err != nil {
				return fmt.Errorf("commit created file %s: %w", mutation.path, err)
			}
			mutation.staged = ""
			mutation.committed = true
		case "update", "delete":
			mutation.backup, err = reservePatchPath(filepath.Dir(mutation.target), ".omnidex-patch-backup-*")
			if err != nil {
				return fmt.Errorf("reserve patch backup for %s: %w", mutation.path, err)
			}
			if err = os.Rename(mutation.target, mutation.backup); err != nil {
				return fmt.Errorf("backup patch target %s: %w", mutation.path, err)
			}
			mutation.originalMoved = true
			if mutation.action == "update" {
				if err = os.Rename(mutation.staged, mutation.target); err != nil {
					return fmt.Errorf("commit updated file %s: %w", mutation.path, err)
				}
				mutation.staged = ""
			}
			mutation.committed = true
		default:
			return fmt.Errorf("unsupported patch action %q", mutation.action)
		}
	}
	if err = ctx.Err(); err != nil {
		return fmt.Errorf("commit patch: %w", err)
	}
	rollbackRequired = false
	if err = removePatchBackups(mutations); err != nil {
		return fmt.Errorf("patch committed but backup cleanup failed: %w", err)
	}
	return nil
}

func stagePatchMutations(ctx context.Context, mutations []patchMutation) error {
	for index := range mutations {
		mutation := &mutations[index]
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("stage patch: %w", err)
		}
		if mutation.action == "delete" {
			continue
		}
		file, err := os.CreateTemp(filepath.Dir(mutation.target), ".omnidex-patch-stage-*")
		if err != nil {
			return fmt.Errorf("stage patch target %s: %w", mutation.path, err)
		}
		mutation.staged = file.Name()
		if err := file.Chmod(mutation.mode); err != nil {
			_ = file.Close()
			return fmt.Errorf("set staged patch mode %s: %w", mutation.path, err)
		}
		if _, err := file.Write(mutation.next); err != nil {
			_ = file.Close()
			return fmt.Errorf("write staged patch %s: %w", mutation.path, err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return fmt.Errorf("sync staged patch %s: %w", mutation.path, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close staged patch %s: %w", mutation.path, err)
		}
	}
	return nil
}

func verifyPatchTargetUnchanged(mutation patchMutation) error {
	if mutation.action == "create" {
		if _, err := os.Lstat(mutation.target); err == nil {
			return fmt.Errorf("patch target changed after validation: %s now exists", mutation.path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("recheck patch target %s: %w", mutation.path, err)
		}
		return nil
	}
	current, err := os.ReadFile(mutation.target)
	if err != nil {
		return fmt.Errorf("recheck patch target %s: %w", mutation.path, err)
	}
	if !bytes.Equal(current, mutation.original) {
		return fmt.Errorf("patch target changed after validation: %s", mutation.path)
	}
	return nil
}

func rollbackPatchMutations(mutations []patchMutation) error {
	var rollbackErr error
	for index := len(mutations) - 1; index >= 0; index-- {
		mutation := mutations[index]
		if mutation.committed && mutation.action != "delete" {
			if err := os.Remove(mutation.target); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove superseded patch target %s: %w", mutation.path, err))
			}
		}
		if mutation.originalMoved && mutation.backup != "" {
			if err := os.Rename(mutation.backup, mutation.target); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore patch target %s: %w", mutation.path, err))
			}
		}
		if mutation.staged != "" {
			if err := os.Remove(mutation.staged); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove staged patch %s: %w", mutation.path, err))
			}
		}
	}
	return rollbackErr
}

func removePatchBackups(mutations []patchMutation) error {
	var cleanupErr error
	for _, mutation := range mutations {
		if mutation.backup == "" {
			continue
		}
		if err := os.Remove(mutation.backup); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove patch backup for %s: %w", mutation.path, err))
		}
	}
	return cleanupErr
}

func reservePatchPath(directory, pattern string) (string, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func createPatchParentDirectories(workspace string, mutations []patchMutation) ([]string, error) {
	needed := map[string]struct{}{}
	for _, mutation := range mutations {
		if mutation.action == "delete" {
			continue
		}
		for current := filepath.Dir(mutation.target); current != workspace; current = filepath.Dir(current) {
			if current == "." || current == string(filepath.Separator) {
				return nil, fmt.Errorf("patch parent escaped workspace for %s", mutation.path)
			}
			if _, err := os.Stat(current); err == nil {
				break
			} else if !os.IsNotExist(err) {
				return nil, fmt.Errorf("inspect patch parent for %s: %w", mutation.path, err)
			}
			needed[current] = struct{}{}
		}
	}
	directories := make([]string, 0, len(needed))
	for directory := range needed {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(i, j int) bool {
		return strings.Count(directories[i], string(filepath.Separator)) < strings.Count(directories[j], string(filepath.Separator))
	})
	created := make([]string, 0, len(directories))
	for _, directory := range directories {
		if err := os.Mkdir(directory, 0o755); err != nil {
			return created, errors.Join(fmt.Errorf("create patch parent %s: %w", directory, err), removePatchDirectories(created))
		}
		created = append(created, directory)
	}
	return created, nil
}

func removePatchDirectories(directories []string) error {
	var cleanupErr error
	for index := len(directories) - 1; index >= 0; index-- {
		if err := os.Remove(directories[index]); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove patch directory %s: %w", directories[index], err))
		}
	}
	return cleanupErr
}
