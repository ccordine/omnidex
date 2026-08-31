package workspace

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
	"time"
)

func Capture(ctx context.Context, requestedRoot string, selectedPaths []string) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, fmt.Errorf("workspace path capture requires a context")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("workspace path capture: %w", err)
	}
	root, before, err := exactRootDirectory(requestedRoot)
	if err != nil {
		return Snapshot{}, err
	}
	paths := append([]string(nil), selectedPaths...)
	sort.Strings(paths)
	for index, relative := range paths {
		if err := validateRelativePath(relative); err != nil {
			return Snapshot{}, err
		}
		if index > 0 && paths[index-1] == relative {
			return Snapshot{}, fmt.Errorf("workspace path capture repeats %q", relative)
		}
	}
	snapshot := Snapshot{
		Schema: SnapshotSchemaV3, WorkspaceID: workspaceID(root), Root: root,
		GeneratedAt: canonicalTime(time.Now()), Paths: paths, Entries: make([]Entry, 0, len(paths)),
	}
	for _, relative := range paths {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, fmt.Errorf("workspace path capture: %w", err)
		}
		entry, present, err := inspectSelectedEntry(ctx, root, relative)
		if err != nil {
			return Snapshot{}, err
		}
		if present {
			snapshot.Entries = append(snapshot.Entries, entry)
		}
	}
	after, err := os.Lstat(root)
	if err != nil || !after.IsDir() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) {
		return Snapshot{}, fmt.Errorf("workspace root changed while selected paths were captured")
	}
	return sealSnapshot(snapshot)
}

func inspectSelectedEntry(ctx context.Context, root, relative string) (Entry, bool, error) {
	current := root
	parts := strings.Split(relative, "/")
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return Entry{}, false, nil
		}
		if err != nil {
			return Entry{}, false, fmt.Errorf("inspect parent for workspace path %q: %w", relative, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Entry{}, false, nil
		}
	}
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	before, err := os.Lstat(absolute)
	if os.IsNotExist(err) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("inspect workspace path %q: %w", relative, err)
	}
	entry := Entry{
		ID: opaqueID("workspace_file_", workspaceID(root), relative), Path: relative,
		Mode: uint32(before.Mode().Perm()),
	}
	switch {
	case before.Mode().IsRegular():
		digest, err := hashFile(ctx, absolute)
		if err != nil {
			return Entry{}, false, fmt.Errorf("hash workspace path %q: %w", relative, err)
		}
		after, err := os.Lstat(absolute)
		if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) ||
			before.Size() != after.Size() || before.Mode() != after.Mode() ||
			!before.ModTime().Equal(after.ModTime()) {
			return Entry{}, false, fmt.Errorf("workspace path %q changed while captured", relative)
		}
		entry.Kind, entry.SHA256, entry.Size = EntryRegular, digest, before.Size()
	case before.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(absolute)
		if err != nil {
			return Entry{}, false, fmt.Errorf("read workspace symlink %q: %w", relative, err)
		}
		entry.Kind, entry.LinkTarget = EntrySymlink, target
		entry.Size, entry.SHA256 = int64(len(target)), digestBytes([]byte("symlink\x00"+target))
	case before.IsDir():
		entry.Kind, entry.SHA256 = EntryDirectory, digestBytes([]byte("directory\x00"))
	default:
		return Entry{}, false, fmt.Errorf("workspace path %q has an unsupported filesystem kind", relative)
	}
	return entry, true, nil
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
