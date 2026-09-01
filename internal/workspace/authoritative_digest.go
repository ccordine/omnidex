package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// WorkspaceDigestOptions bounds one root-relative authoritative workspace
// snapshot. ExcludedRootNames are exact first-level names owned by deterministic
// tool output rather than source authority.
type WorkspaceDigestOptions struct {
	ExcludedRootNames []string
	MaxPaths          int
	MaxBytes          int64
}

type authoritativeWorkspaceDigester struct {
	root      *authoritativeWorkspaceRoot
	excluded  map[string]struct{}
	maximum   WorkspaceDigestOptions
	digest    hash.Hash
	paths     int
	bytesRead int64
}

// AuthoritativeWorkspaceSHA256 snapshots exactRoot through the immutable root
// handle owned by the fence. The fence remains held for the complete snapshot.
func (fence *MutationFence) AuthoritativeWorkspaceSHA256(
	exactRoot string,
	options WorkspaceDigestOptions,
) (string, error) {
	excluded, err := validateWorkspaceDigestOptions(options)
	if err != nil {
		return "", err
	}
	if fence == nil {
		return "", fmt.Errorf("authoritative workspace digest requires one mutation fence")
	}
	fence.mu.Lock()
	defer fence.mu.Unlock()
	if exactRoot == "" || exactRoot != fence.root {
		return "", fmt.Errorf("authoritative workspace digest root differs from its mutation fence")
	}
	root, err := fence.authoritativeRootLocked()
	if err != nil {
		return "", fmt.Errorf("attest authoritative workspace before digest: %w", err)
	}
	digester := authoritativeWorkspaceDigester{
		root: root, excluded: excluded, maximum: options, digest: sha256.New(),
	}
	rootInfo, err := root.Lstat(".")
	if err != nil {
		return "", fmt.Errorf("inspect authoritative workspace root: %w", err)
	}
	if err := digester.digestDirectory(".", rootInfo, true); err != nil {
		return "", err
	}
	if _, err := fence.authoritativeRootLocked(); err != nil {
		return "", fmt.Errorf("reattest authoritative workspace after digest: %w", err)
	}
	return hex.EncodeToString(digester.digest.Sum(nil)), nil
}

func validateWorkspaceDigestOptions(options WorkspaceDigestOptions) (map[string]struct{}, error) {
	if options.MaxPaths < 1 || options.MaxBytes < 1 {
		return nil, fmt.Errorf("workspace digest requires positive path and byte bounds")
	}
	excluded := make(map[string]struct{}, len(options.ExcludedRootNames))
	for _, name := range options.ExcludedRootNames {
		if name == "" || name == "." || name == ".." || path.Base(name) != name ||
			!utf8.ValidString(name) || strings.ContainsRune(name, 0) {
			return nil, fmt.Errorf("workspace digest excluded root %q is invalid", name)
		}
		if _, duplicate := excluded[name]; duplicate {
			return nil, fmt.Errorf("workspace digest repeats excluded root %q", name)
		}
		excluded[name] = struct{}{}
	}
	return excluded, nil
}

func (digester *authoritativeWorkspaceDigester) digestDirectory(
	relative string,
	expected os.FileInfo,
	isRoot bool,
) (resultErr error) {
	directory, opened, err := digester.openBoundPath(
		relative, expected, unix.O_DIRECTORY,
	)
	if err != nil {
		return fmt.Errorf("open authoritative workspace directory %q: %w", relative, err)
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	entries, err := digester.readDirectoryEntries(directory, isRoot)
	if err != nil {
		return fmt.Errorf("read authoritative workspace directory %q: %w", relative, err)
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name == "." || name == ".." || !utf8.ValidString(name) ||
			strings.ContainsRune(name, 0) || strings.Contains(name, "/") {
			return fmt.Errorf("authoritative workspace contains an invalid basename")
		}
		if isRoot {
			if _, skip := digester.excluded[name]; skip {
				if err := digester.validateExcludedRoot(name); err != nil {
					return err
				}
				continue
			}
		}
		candidate := name
		if relative != "." {
			candidate = path.Join(relative, name)
		}
		if err := digester.digestPath(candidate); err != nil {
			return err
		}
	}
	openedAfter, err := directory.Stat()
	if err != nil {
		return fmt.Errorf("restat authoritative workspace directory %q: %w", relative, err)
	}
	return digester.restatBoundPath(relative, expected, opened, openedAfter)
}

func (digester *authoritativeWorkspaceDigester) readDirectoryEntries(
	directory *os.File,
	isRoot bool,
) ([]os.DirEntry, error) {
	remaining := digester.maximum.MaxPaths - digester.paths
	if remaining < 0 {
		return nil, fmt.Errorf(
			"authoritative workspace digest exceeds %d paths",
			digester.maximum.MaxPaths,
		)
	}
	if isRoot {
		remaining += len(digester.excluded)
	}
	entries := make([]os.DirEntry, 0, min(remaining, 256))
	for {
		batch, readErr := directory.ReadDir(256)
		if len(batch) == 0 && readErr == nil {
			return nil, fmt.Errorf("directory read made no bounded progress")
		}
		if len(entries) > remaining-len(batch) {
			return nil, fmt.Errorf(
				"authoritative workspace digest exceeds %d paths",
				digester.maximum.MaxPaths,
			)
		}
		entries = append(entries, batch...)
		if errors.Is(readErr, io.EOF) {
			return entries, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

// validateExcludedRoot proves that an ignored tool-output root is still one
// exact regular entry on the authoritative mount. Its bytes and descendants
// remain deliberately outside the source digest.
func (digester *authoritativeWorkspaceDigester) validateExcludedRoot(relative string) error {
	expected, err := digester.root.Lstat(relative)
	if err != nil {
		return fmt.Errorf("inspect excluded authoritative workspace root %q: %w", relative, err)
	}
	if expected.Mode()&os.ModeSymlink != 0 ||
		(!expected.IsDir() && !expected.Mode().IsRegular()) {
		return fmt.Errorf(
			"authoritative workspace digest rejects non-regular excluded root %q",
			relative,
		)
	}
	typeFlag := 0
	if expected.IsDir() {
		typeFlag = unix.O_DIRECTORY
	}
	handle, opened, err := digester.openBoundPath(relative, expected, typeFlag)
	if err != nil {
		return fmt.Errorf("open excluded authoritative workspace root %q: %w", relative, err)
	}
	openedAfter, statErr := handle.Stat()
	closeErr := handle.Close()
	if statErr != nil {
		return errors.Join(
			fmt.Errorf("restat excluded authoritative workspace root %q: %w", relative, statErr),
			closeErr,
		)
	}
	if err := digester.restatBoundPath(relative, expected, opened, openedAfter); err != nil {
		return errors.Join(err, closeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close excluded authoritative workspace root %q: %w", relative, closeErr)
	}
	return nil
}

func (digester *authoritativeWorkspaceDigester) digestPath(relative string) error {
	info, err := digester.root.Lstat(relative)
	if err != nil {
		return fmt.Errorf("inspect authoritative workspace path %q: %w", relative, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return fmt.Errorf("authoritative workspace digest rejects non-regular path %q", relative)
	}
	digester.paths++
	if digester.paths > digester.maximum.MaxPaths {
		return fmt.Errorf("authoritative workspace digest exceeds %d paths", digester.maximum.MaxPaths)
	}
	kind := "directory"
	if info.Mode().IsRegular() {
		kind = "file"
	}
	writeWorkspaceDigestField(digester.digest, kind)
	writeWorkspaceDigestField(digester.digest, relative)
	writeWorkspaceDigestField(digester.digest, strconv.FormatUint(uint64(info.Mode().Perm()), 8))
	if info.IsDir() {
		return digester.digestDirectory(relative, info, false)
	}
	if info.Size() < 0 || digester.bytesRead > digester.maximum.MaxBytes-info.Size() {
		return fmt.Errorf("authoritative workspace digest exceeds %d content bytes", digester.maximum.MaxBytes)
	}
	writeWorkspaceDigestField(digester.digest, strconv.FormatInt(info.Size(), 10))
	return digester.digestRegularFile(relative, info)
}

func (digester *authoritativeWorkspaceDigester) digestRegularFile(
	relative string,
	expected os.FileInfo,
) (resultErr error) {
	file, opened, err := digester.openBoundPath(relative, expected, 0)
	if err != nil {
		return fmt.Errorf("open authoritative workspace file %q: %w", relative, err)
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	copied, err := io.CopyN(digester.digest, file, expected.Size())
	if err != nil {
		return fmt.Errorf("read authoritative workspace file %q: %w", relative, err)
	}
	if copied != expected.Size() {
		return fmt.Errorf("authoritative workspace file %q changed while hashing", relative)
	}
	var extra [1]byte
	count, readErr := file.Read(extra[:])
	if count != 0 || !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("authoritative workspace file %q changed while hashing", relative)
	}
	openedAfter, err := file.Stat()
	if err != nil {
		return fmt.Errorf("restat authoritative workspace file %q: %w", relative, err)
	}
	if err := digester.restatBoundPath(relative, expected, opened, openedAfter); err != nil {
		return err
	}
	digester.bytesRead += copied
	return nil
}

func (digester *authoritativeWorkspaceDigester) openBoundPath(
	relative string,
	expected os.FileInfo,
	typeFlag int,
) (*os.File, os.FileInfo, error) {
	flags := os.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK | typeFlag
	file, err := digester.root.OpenFile(relative, flags, 0)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil || !sameWorkspaceFileSnapshot(expected, opened) {
		if err == nil {
			err = fmt.Errorf("opened path differs from its exact lstat authority")
		}
		return nil, nil, errors.Join(err, file.Close())
	}
	return file, opened, nil
}

func (digester *authoritativeWorkspaceDigester) restatBoundPath(
	relative string,
	expected os.FileInfo,
	openedBefore os.FileInfo,
	openedAfter os.FileInfo,
) error {
	current, err := digester.root.Lstat(relative)
	if err != nil || !sameWorkspaceFileSnapshot(expected, current) {
		return fmt.Errorf("authoritative workspace path %q changed while hashing", relative)
	}
	if !sameWorkspaceFileSnapshot(expected, openedBefore) ||
		!sameWorkspaceFileSnapshot(expected, openedAfter) {
		return fmt.Errorf("authoritative workspace path %q opened with changed metadata", relative)
	}
	return nil
}

func sameWorkspaceFileSnapshot(expected os.FileInfo, observed os.FileInfo) bool {
	return expected != nil && observed != nil && os.SameFile(expected, observed) &&
		expected.Mode() == observed.Mode() && expected.Size() == observed.Size() &&
		expected.ModTime().Equal(observed.ModTime())
}

func writeWorkspaceDigestField(destination hash.Hash, value string) {
	_, _ = io.WriteString(destination, strconv.Itoa(len(value)))
	_, _ = io.WriteString(destination, ":")
	_, _ = io.WriteString(destination, value)
	_, _ = io.WriteString(destination, "\x00")
}
