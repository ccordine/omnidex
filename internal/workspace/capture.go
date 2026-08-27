package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func Capture(ctx context.Context, requestedRoot string) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, fmt.Errorf("workspace capture requires a context")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("workspace capture: %w", err)
	}
	root, before, err := exactRootDirectory(requestedRoot)
	if err != nil {
		return Snapshot{}, err
	}
	gitMarker := filepath.Join(root, ".git")
	if _, err := os.Lstat(gitMarker); err == nil {
		repositorySnapshot, captureErr := repositoryfacts.BuildGitSnapshot(
			ctx, root, repositoryfacts.SnapshotOptions{MaxFiles: maxSnapshotEntries},
		)
		if captureErr != nil {
			return Snapshot{}, fmt.Errorf("capture root-local Git workspace: %w", captureErr)
		}
		return FromRepositorySnapshot(repositorySnapshot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, fmt.Errorf("inspect root-local Git authority: %w", err)
	}
	snapshot, err := capturePlain(ctx, root)
	if err != nil {
		return Snapshot{}, err
	}
	after, err := os.Lstat(root)
	if err != nil || !after.IsDir() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) {
		return Snapshot{}, fmt.Errorf("workspace root changed while it was captured")
	}
	if _, err := os.Lstat(gitMarker); err == nil {
		return Snapshot{}, fmt.Errorf("root-local Git authority appeared while the plain workspace was captured")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, fmt.Errorf("recheck root-local Git authority: %w", err)
	}
	return snapshot, nil
}

func FromRepositorySnapshot(source repositoryfacts.Snapshot) (Snapshot, error) {
	if err := source.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("workspace Git binding source: %w", err)
	}
	root, err := exactRootPath(source.Root)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{
		Schema: SnapshotSchemaV1, WorkspaceID: workspaceID(root), Root: root,
		GeneratedAt: canonicalTime(source.GeneratedAt),
		Entries:     make([]Entry, len(source.Files)),
		Exclusions:  make([]Exclusion, len(source.Exclusions)),
		Git: &GitBinding{
			RepositorySnapshotID: source.ID, RepositoryID: source.RepositoryID,
			HeadCommit: source.HeadCommit, StateSHA256: source.GitStateSHA256,
		},
	}
	for index, file := range source.Files {
		kind := EntryRegular
		if file.Kind == repositoryfacts.EntrySymlink {
			kind = EntrySymlink
		}
		snapshot.Entries[index] = Entry{
			ID:   opaqueID("workspace_file_", snapshot.WorkspaceID, file.Path),
			Path: file.Path, Kind: kind, SHA256: file.SHA256, Size: file.Size,
			Mode: file.Mode, LinkTarget: file.LinkTarget, RepositoryFileID: file.ID,
		}
	}
	for index, exclusion := range source.Exclusions {
		reason, err := repositoryExclusionReason(exclusion.Reason)
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Exclusions[index] = Exclusion{Path: exclusion.Path, Reason: reason}
	}
	return sealSnapshot(snapshot)
}

func capturePlain(ctx context.Context, root string) (Snapshot, error) {
	snapshot := Snapshot{
		Schema: SnapshotSchemaV1, WorkspaceID: workspaceID(root), Root: root,
		GeneratedAt: canonicalTime(time.Now()), Entries: make([]Entry, 0),
		Exclusions: make([]Exclusion, 0),
	}
	err := filepath.WalkDir(root, func(absolute string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk workspace path %q: %w", absolute, walkErr)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if absolute == root {
			return nil
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return fmt.Errorf("derive workspace-relative path: %w", err)
		}
		relative = filepath.ToSlash(relative)
		if err := validateRelativePath(relative); err != nil {
			return err
		}
		if protectedPath(relative) {
			snapshot.Exclusions = append(snapshot.Exclusions, Exclusion{
				Path: relative, Reason: ExclusionProtected,
			})
			if item.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if item.IsDir() {
			return nil
		}
		if sensitivePath(relative) {
			snapshot.Exclusions = append(snapshot.Exclusions, Exclusion{
				Path: relative, Reason: ExclusionSensitive,
			})
			return nil
		}
		entry, err := inspectPlainEntry(ctx, root, relative)
		if err != nil {
			return err
		}
		snapshot.Entries = append(snapshot.Entries, entry)
		if len(snapshot.Entries) > maxSnapshotEntries {
			return fmt.Errorf("workspace inventory exceeds %d entries", maxSnapshotEntries)
		}
		return nil
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("capture plain workspace: %w", err)
	}
	return sealSnapshot(snapshot)
}

func inspectPlainEntry(ctx context.Context, root, relative string) (Entry, error) {
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	before, err := os.Lstat(absolute)
	if err != nil {
		return Entry{}, fmt.Errorf("inspect workspace entry %q: %w", relative, err)
	}
	entry := Entry{
		ID:   opaqueID("workspace_file_", workspaceID(root), relative),
		Path: relative, Size: before.Size(), Mode: uint32(before.Mode().Perm()),
	}
	switch {
	case before.Mode().IsRegular():
		digest, err := hashFile(ctx, absolute)
		if err != nil {
			return Entry{}, fmt.Errorf("hash workspace entry %q: %w", relative, err)
		}
		after, err := os.Lstat(absolute)
		if err != nil || !after.Mode().IsRegular() || before.Size() != after.Size() ||
			before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
			return Entry{}, fmt.Errorf("workspace entry %q changed while it was captured", relative)
		}
		entry.Kind, entry.SHA256 = EntryRegular, digest
	case before.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(absolute)
		if err != nil {
			return Entry{}, fmt.Errorf("read workspace symlink %q: %w", relative, err)
		}
		if err := validateSymlink(relative, target); err != nil {
			return Entry{}, err
		}
		digest := sha256.Sum256([]byte("symlink\x00" + target))
		entry.Kind, entry.LinkTarget = EntrySymlink, target
		entry.Size, entry.SHA256 = int64(len(target)), hex.EncodeToString(digest[:])
	default:
		return Entry{}, fmt.Errorf("workspace entry %q has unsupported filesystem kind", relative)
	}
	return entry, nil
}

func hashFile(ctx context.Context, absolute string) (string, error) {
	file, err := os.Open(absolute)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, &contextReader{ctx: ctx, reader: file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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

func sealSnapshot(snapshot Snapshot) (Snapshot, error) {
	sortSnapshot(&snapshot)
	id, err := snapshotID(snapshot)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.ID = id
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func repositoryExclusionReason(reason repositoryfacts.ExclusionReason) (ExclusionReason, error) {
	switch reason {
	case repositoryfacts.ExclusionSensitive:
		return ExclusionSensitive, nil
	case repositoryfacts.ExclusionAbsent:
		return ExclusionAbsent, nil
	case repositoryfacts.ExclusionUnsupported:
		return ExclusionUnsupported, nil
	default:
		return "", fmt.Errorf("repository exclusion reason %q is not registered for workspace authority", reason)
	}
}

func sensitivePath(value string) bool {
	lower := strings.ToLower(value)
	base := path.Base(lower)
	if base == ".env.example" {
		return false
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	extension := path.Ext(lower)
	return extension == ".pem" || extension == ".key"
}

func exactRootPath(requested string) (string, error) {
	if requested == "" || requested != strings.TrimSpace(requested) {
		return "", fmt.Errorf("workspace root must be non-empty canonical text")
	}
	if !filepath.IsAbs(requested) {
		return "", fmt.Errorf("workspace root must be one canonical absolute path")
	}
	root, err := filepath.Abs(requested)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	root = filepath.Clean(root)
	if requested != root {
		return "", fmt.Errorf("workspace root must be one canonical absolute path")
	}
	return root, nil
}

func exactRootDirectory(requested string) (string, os.FileInfo, error) {
	root, err := exactRootPath(requested)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("workspace root %q is absent or not one exact directory", root)
	}
	return root, info, nil
}
