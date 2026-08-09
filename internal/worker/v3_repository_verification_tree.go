package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type repositoryVerificationTree map[string]repositoryVerificationTreeEntry

type repositoryVerificationTreeEntry struct {
	Kind   string
	Mode   os.FileMode
	Size   int64
	Digest string
}

func captureRepositoryVerificationTree(root string) (repositoryVerificationTree, error) {
	return captureRepositoryVerificationTreeContext(context.Background(), root)
}

func captureRepositoryVerificationTreeContext(
	ctx context.Context,
	root string,
) (repositoryVerificationTree, error) {
	return captureRepositoryVerificationTreeContextWithLimits(ctx, root, 0, 0)
}

func captureRepositoryVerificationTreeContextWithLimits(
	ctx context.Context,
	root string,
	maxEntries int,
	maxRegularBytes int64,
) (repositoryVerificationTree, error) {
	if ctx == nil {
		return nil, fmt.Errorf("repository verification tree requires a context")
	}
	rootInfo, err := exactRepositoryVerificationRoot(root)
	if err != nil {
		return nil, err
	}
	state := make(repositoryVerificationTree)
	var regularBytes int64
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("repository verification tree authority ended: %w", err)
		}
		if walkErr != nil {
			return fmt.Errorf("walk repository verification tree: %w", walkErr)
		}
		if maxEntries > 0 && len(state) >= maxEntries {
			return fmt.Errorf("repository Go module view exceeds exact %d-entry limit", maxEntries)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve repository verification entry: %w", err)
		}
		if relative == "." {
			state["."] = repositoryVerificationTreeEntry{Kind: "directory", Mode: rootInfo.Mode()}
			return nil
		}
		relative = filepath.ToSlash(relative)
		if relative == ".git" || relative == ".omni" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect repository verification entry %q: %w", relative, err)
		}
		item := repositoryVerificationTreeEntry{Mode: info.Mode(), Size: info.Size()}
		switch {
		case info.IsDir():
			item.Kind = "directory"
		case info.Mode().IsRegular():
			item.Kind = "regular"
			if maxRegularBytes > 0 && (info.Size() < 0 || regularBytes > maxRegularBytes-info.Size()) {
				return fmt.Errorf(
					"repository Go module view exceeds exact %d-regular-byte limit",
					maxRegularBytes,
				)
			}
			regularBytes += info.Size()
			item.Digest, err = hashRepositoryVerificationFile(ctx, path, info)
		case info.Mode()&os.ModeSymlink != 0:
			item.Kind = "symlink"
			var target string
			target, err = os.Readlink(path)
			if err == nil {
				sum := sha256.Sum256([]byte("symlink\x00" + target))
				item.Digest = hex.EncodeToString(sum[:])
			}
		default:
			return fmt.Errorf(
				"repository verification entry %q has unsupported mode %s",
				relative, info.Mode(),
			)
		}
		if err != nil {
			return fmt.Errorf("inspect repository verification entry %q: %w", relative, err)
		}
		state[relative] = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

func assertRepositoryVerificationTreeUnchanged(
	root string,
	before repositoryVerificationTree,
) error {
	return assertRepositoryVerificationTreeUnchangedContext(context.Background(), root, before)
}

func assertRepositoryVerificationTreeUnchangedContext(
	ctx context.Context,
	root string,
	before repositoryVerificationTree,
) error {
	return assertRepositoryVerificationTreeUnchangedContextWithLimits(ctx, root, before, 0, 0)
}

func assertRepositoryVerificationTreeUnchangedContextWithLimits(
	ctx context.Context,
	root string,
	before repositoryVerificationTree,
	maxEntries int,
	maxRegularBytes int64,
) error {
	if len(before) == 0 {
		return fmt.Errorf("repository verification requires one non-empty pre-command tree authority")
	}
	after, err := captureRepositoryVerificationTreeContextWithLimits(
		ctx, root, maxEntries, maxRegularBytes,
	)
	if err != nil {
		return err
	}
	paths := make(map[string]struct{}, len(before)+len(after))
	for path := range before {
		paths[path] = struct{}{}
	}
	for path := range after {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		prior, hadPrior := before[path]
		current, hasCurrent := after[path]
		if !hadPrior {
			return fmt.Errorf("repository verification created unexpected entry %q", path)
		}
		if !hasCurrent {
			return fmt.Errorf("repository verification removed expected entry %q", path)
		}
		if prior != current {
			return fmt.Errorf("repository verification changed exact entry %q", path)
		}
	}
	return nil
}

func exactRepositoryVerificationRoot(root string) (os.FileInfo, error) {
	if root == "" || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("repository verification requires one absolute root")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("repository verification root is not one exact directory")
	}
	for _, forbidden := range []string{".git", ".omni"} {
		if _, err := os.Lstat(filepath.Join(root, forbidden)); err == nil {
			return nil, fmt.Errorf(
				"repository verification requires a snapshot-only workspace without %s", forbidden,
			)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("inspect repository verification %s boundary: %w", forbidden, err)
		}
	}
	return info, nil
}

func hashRepositoryVerificationFile(
	ctx context.Context,
	path string,
	before os.FileInfo,
) (string, error) {
	handle, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer handle.Close()
	opened, err := handle.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return "", fmt.Errorf("file changed while it was opened")
	}
	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		count, readErr := handle.Read(buffer)
		if count > 0 {
			if _, err := hash.Write(buffer[:count]); err != nil {
				return "", err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() ||
		before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("file changed while it was hashed")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
