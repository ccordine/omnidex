package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxGitMetadataBytes = 256 * 1024 * 1024

type SnapshotOptions struct {
	MaxFiles int
	GitBin   string
}

func BuildGitSnapshot(ctx context.Context, root string, options SnapshotOptions) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, fmt.Errorf("repository snapshot requires a context")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("repository snapshot: %w", err)
	}
	if options.MaxFiles < 0 {
		return Snapshot{}, fmt.Errorf("repository snapshot max files cannot be negative")
	}
	root, err := exactGitRoot(ctx, root, options.GitBin)
	if err != nil {
		return Snapshot{}, err
	}
	gitBin := normalizedGitBin(options.GitBin)
	head, err := runGitText(ctx, gitBin, root, "rev-parse", "HEAD")
	if err != nil {
		return Snapshot{}, fmt.Errorf("repository snapshot requires a committed Git HEAD: %w", err)
	}
	if !validGitOID(head) {
		return Snapshot{}, fmt.Errorf("repository snapshot returned invalid Git HEAD object ID %q", head)
	}
	commonDir, err := runGitText(ctx, gitBin, root, "rev-parse", "--git-common-dir")
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve repository Git identity: %w", err)
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	commonDir, err = filepath.Abs(commonDir)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve repository Git directory: %w", err)
	}
	repositoryID := opaqueID("repository_", filepath.Clean(commonDir))

	listed, err := runGitBytes(
		ctx, gitBin, root,
		"ls-files", "-z", "--cached", "--others", "--exclude-standard",
		"--", ".", ":(exclude).omni", ":(exclude).omni/**",
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("inventory repository files: %w", err)
	}
	paths, err := decodeGitPaths(listed)
	if err != nil {
		return Snapshot{}, err
	}
	if options.MaxFiles > 0 && len(paths) > options.MaxFiles {
		return Snapshot{}, fmt.Errorf(
			"repository index incomplete: discovered %d entries exceeds configured hard limit %d",
			len(paths), options.MaxFiles,
		)
	}

	status, err := runGitBytes(
		ctx, gitBin, root,
		"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=no",
		"--", ".", ":(exclude).omni", ":(exclude).omni/**",
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("capture repository Git state: %w", err)
	}
	stateHash := sha256.Sum256(status)
	snapshot := Snapshot{
		Schema: SnapshotSchemaV1, RepositoryID: repositoryID, Root: root,
		HeadCommit: head, GitStateSHA256: hex.EncodeToString(stateHash[:]),
		Dirty: len(status) > 0, GeneratedAt: canonicalRepositoryTimestamp(time.Now()),
		Files: make([]File, 0, len(paths)), Exclusions: make([]Exclusion, 0),
	}
	for _, relative := range paths {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, fmt.Errorf("repository snapshot: %w", err)
		}
		if err := validateRelativeRepositoryPath(relative); err != nil {
			return Snapshot{}, fmt.Errorf("Git returned an unsupported repository path: %w", err)
		}
		if sensitiveRepositoryPath(relative) {
			snapshot.Exclusions = append(snapshot.Exclusions, Exclusion{Path: relative, Reason: ExclusionSensitive})
			continue
		}
		file, exclusion, err := inspectRepositoryEntry(root, repositoryID, relative)
		if err != nil {
			return Snapshot{}, err
		}
		if exclusion != nil {
			snapshot.Exclusions = append(snapshot.Exclusions, *exclusion)
			continue
		}
		snapshot.Files = append(snapshot.Files, file)
	}
	sortSnapshotFacts(&snapshot)
	snapshot.ID, err = snapshotID(snapshot)
	if err != nil {
		return Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func exactGitRoot(ctx context.Context, requested, gitBin string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", fmt.Errorf("repository snapshot root is required")
	}
	abs, err := filepath.Abs(requested)
	if err != nil {
		return "", fmt.Errorf("resolve repository snapshot root %q: %w", requested, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("inspect repository snapshot root %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository snapshot root %q is not a directory", abs)
	}
	top, err := runGitText(ctx, normalizedGitBin(gitBin), abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repository snapshot requires a Git worktree: %w", err)
	}
	top, err = filepath.Abs(top)
	if err != nil {
		return "", fmt.Errorf("resolve Git worktree root: %w", err)
	}
	if filepath.Clean(abs) != filepath.Clean(top) {
		return "", fmt.Errorf("repository snapshot root %q must equal Git worktree root %q", abs, top)
	}
	return filepath.Clean(top), nil
}

func inspectRepositoryEntry(root, repositoryID, relative string) (File, *Exclusion, error) {
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	before, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, &Exclusion{Path: relative, Reason: ExclusionAbsent}, nil
	}
	if err != nil {
		return File{}, nil, fmt.Errorf("inspect repository entry %q: %w", relative, err)
	}
	file := File{
		ID: opaqueID("file_", repositoryID, relative), Path: relative,
		Size: before.Size(), Mode: uint32(before.Mode().Perm()),
		Language: languageForRepositoryPath(relative), Manifest: manifestForRepositoryPath(relative),
		Test: testRepositoryPath(relative), Generated: generatedRepositoryPath(relative),
	}
	switch {
	case before.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(absolute)
		if err != nil {
			return File{}, nil, fmt.Errorf("read repository symlink %q: %w", relative, err)
		}
		hash := sha256.Sum256([]byte("symlink\x00" + target))
		file.Kind = EntrySymlink
		file.LinkTarget = target
		file.SHA256 = hex.EncodeToString(hash[:])
		file.Size = int64(len(target))
		return file, nil, nil
	case before.Mode().IsRegular():
		digest, err := hashRepositoryFile(absolute)
		if err != nil {
			return File{}, nil, fmt.Errorf("hash repository file %q: %w", relative, err)
		}
		after, err := os.Lstat(absolute)
		if err != nil {
			return File{}, nil, fmt.Errorf("recheck repository file %q: %w", relative, err)
		}
		if !after.Mode().IsRegular() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return File{}, nil, fmt.Errorf("repository file %q changed while it was being indexed", relative)
		}
		file.Kind = EntryRegular
		file.SHA256 = digest
		return file, nil, nil
	default:
		return File{}, &Exclusion{Path: relative, Reason: ExclusionUnsupported}, nil
	}
}

func hashRepositoryFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func decodeGitPaths(raw []byte) ([]string, error) {
	parts := strings.Split(string(raw), "\x00")
	paths := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, value := range parts {
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("Git repository inventory returned duplicate path %q", value)
		}
		seen[value] = struct{}{}
		paths = append(paths, value)
	}
	sort.Strings(paths)
	return paths, nil
}

func normalizedGitBin(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "git"
	}
	return value
}

func runGitText(ctx context.Context, gitBin, root string, args ...string) (string, error) {
	raw, err := runGitBytes(ctx, gitBin, root, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func runGitBytes(ctx context.Context, gitBin, root string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, gitBin, append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git %s failed: %s", args[0], boundedGitError(exitErr.Stderr))
		}
		return nil, fmt.Errorf("run git %s: %w", args[0], err)
	}
	if len(output) > maxGitMetadataBytes {
		return nil, fmt.Errorf("git %s output exceeds %d bytes", args[0], maxGitMetadataBytes)
	}
	return output, nil
}

func boundedGitError(raw []byte) string {
	const limit = 2048
	value := strings.TrimSpace(string(raw))
	if len(value) > limit {
		return value[:limit] + "...[truncated]"
	}
	if value == "" {
		return "no diagnostic"
	}
	return value
}
