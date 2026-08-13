package changeapply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/gryph/omnidex/internal/omni"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (stage *StagedChange) applyVerified(ctx context.Context) (omni.PatchApplyResult, error) {
	if ctx == nil {
		return omni.PatchApplyResult{}, fmt.Errorf("apply verified repository change requires a context")
	}
	if err := ctx.Err(); err != nil {
		return omni.PatchApplyResult{}, fmt.Errorf("apply verified repository change: %w", err)
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return omni.PatchApplyResult{}, fmt.Errorf("repository change stage %q is closed", stage.id)
	}
	if stage.applied {
		return omni.PatchApplyResult{}, fmt.Errorf("repository change stage %q was already applied", stage.id)
	}
	if digest([]byte(stage.patch)) != stage.patchSHA256 {
		return omni.PatchApplyResult{}, fmt.Errorf("repository change stage %q patch identity is invalid", stage.id)
	}
	if err := verifyStagedWorkspace(stage.workspace, stage.stagedFiles); err != nil {
		return omni.PatchApplyResult{}, err
	}
	if err := verifyAuthoritativeSnapshot(ctx, stage.authoritativeRoot, stage.expectedSnapshotID); err != nil {
		return omni.PatchApplyResult{}, err
	}
	result, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{
		Context: ctx, Workspace: stage.authoritativeRoot, Patch: stage.patch,
	})
	if err != nil {
		return omni.PatchApplyResult{}, fmt.Errorf("apply verified repository patch: %w", err)
	}
	stage.applied = true
	return result, nil
}

func (stage *StagedChange) Cleanup() error {
	if stage == nil {
		return nil
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return nil
	}
	if err := os.RemoveAll(stage.workspace); err != nil {
		return fmt.Errorf("clean repository change stage %q: %w", stage.id, err)
	}
	stage.closed = true
	return nil
}

func stagedFileAuthorities(snapshot repositoryfacts.Snapshot, mutations []fileMutation) []stagedFileAuthority {
	changed := make(map[string]fileMutation, len(mutations))
	for _, mutation := range mutations {
		changed[mutation.file.Path] = mutation
	}
	authorities := make([]stagedFileAuthority, 0, len(snapshot.Files)+len(mutations))
	for _, file := range snapshot.Files {
		mutation, exists := changed[file.Path]
		if exists && !mutation.desiredPresent {
			delete(changed, file.Path)
			continue
		}
		authority := stagedFileAuthority{
			path: file.Path, kind: file.Kind, sha256: file.SHA256,
			size: file.Size, mode: file.Mode, linkTarget: file.LinkTarget,
		}
		if exists {
			authority.sha256 = digest(mutation.next)
			authority.size = int64(len(mutation.next))
			delete(changed, file.Path)
		}
		authorities = append(authorities, authority)
	}
	for _, mutation := range changed {
		if !mutation.desiredPresent {
			continue
		}
		authorities = append(authorities, stagedFileAuthority{
			path: mutation.file.Path, kind: repositoryfacts.EntryRegular,
			sha256: digest(mutation.next), size: int64(len(mutation.next)), mode: mutation.file.Mode,
		})
	}
	sort.Slice(authorities, func(left, right int) bool { return authorities[left].path < authorities[right].path })
	return authorities
}

func verifyStagedWorkspace(root string, files []stagedFileAuthority) error {
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository change staging workspace is absent or invalid")
	}
	for _, reserved := range []string{".git", ".omni"} {
		if _, err := os.Lstat(filepath.Join(root, reserved)); err == nil {
			return fmt.Errorf("repository change staging workspace contains forbidden %s state", reserved)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect staged %s state: %w", reserved, err)
		}
	}
	if err := verifyExactStagedInventory(root, files); err != nil {
		return err
	}
	for _, file := range files {
		if err := rejectSourceSymlinkParents(root, file.path); err != nil {
			return fmt.Errorf("validate staged repository parent authority: %w", err)
		}
		absolute := filepath.Join(root, filepath.FromSlash(file.path))
		info, err := os.Lstat(absolute)
		if err != nil {
			return fmt.Errorf("inspect staged repository file %q: %w", file.path, err)
		}
		if info.Mode().Perm() != os.FileMode(file.mode) || info.Size() != file.size {
			return fmt.Errorf("staged repository file %q was tampered after planning", file.path)
		}
		switch file.kind {
		case repositoryfacts.EntryRegular:
			if !info.Mode().IsRegular() {
				return fmt.Errorf("staged repository file %q was replaced with an unsupported entry", file.path)
			}
			actual, err := hashExactFile(absolute, info)
			if err != nil {
				return fmt.Errorf("hash staged repository file %q: %w", file.path, err)
			}
			if actual != file.sha256 {
				return fmt.Errorf("staged repository file %q was tampered after planning", file.path)
			}
		case repositoryfacts.EntrySymlink:
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("staged repository symlink %q was replaced", file.path)
			}
			target, err := os.Readlink(absolute)
			if err != nil || target != file.linkTarget || digest([]byte("symlink\x00"+target)) != file.sha256 {
				return fmt.Errorf("staged repository symlink %q was tampered after planning", file.path)
			}
			after, err := os.Lstat(absolute)
			if err != nil || beforeAndAfterDiffer(info, after) {
				return fmt.Errorf("staged repository symlink %q changed while it was verified", file.path)
			}
		default:
			return fmt.Errorf("staged repository file %q has unsupported kind %q", file.path, file.kind)
		}
	}
	return nil
}

func verifyExactStagedInventory(root string, files []stagedFileAuthority) error {
	expected := map[string]bool{".": true}
	for _, file := range files {
		expected[file.path] = false
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(file.path))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			expected[parent] = true
		}
	}
	seen := make(map[string]struct{}, len(expected))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		wantDirectory, exists := expected[relative]
		if !exists || entry.IsDir() != wantDirectory {
			return fmt.Errorf("staged repository contains unexpected inventory entry %q", relative)
		}
		seen[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return fmt.Errorf("verify exact staged repository inventory: %w", err)
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("staged repository exact inventory is incomplete")
	}
	return nil
}

func hashExactFile(path string, before os.FileInfo) (string, error) {
	handle, err := os.Open(path)
	if err != nil {
		return "", err
	}
	opened, err := handle.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = handle.Close()
		return "", fmt.Errorf("file changed while it was opened")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		_ = handle.Close()
		return "", err
	}
	if err := handle.Close(); err != nil {
		return "", err
	}
	after, err := os.Lstat(path)
	if err != nil || beforeAndAfterDiffer(before, after) {
		return "", fmt.Errorf("file changed while it was hashed")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func beforeAndAfterDiffer(before, after os.FileInfo) bool {
	return !os.SameFile(before, after) || before.Size() != after.Size() ||
		!before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode()
}
