package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modulezip "golang.org/x/mod/zip"
)

func projectRepositoryGoCachedModules(
	ctx context.Context,
	viewRoot string,
	hostCache string,
	modules []repositoryGoResolvedModule,
	limits repositoryGoModuleViewLimits,
) error {
	usage := repositoryGoModuleViewUsage{Entries: 1}
	for _, item := range modules {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("project repository Go module view: %w", err)
		}
		planned, err := inspectRepositoryGoCachedModuleUsage(
			ctx, hostCache, item, limits.MaxEntries-usage.Entries,
		)
		if err != nil {
			return err
		}
		if _, err := usage.add(planned, limits); err != nil {
			return err
		}
		usage, err = projectRepositoryGoCachedModule(
			ctx, viewRoot, hostCache, item, usage, limits,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func projectRepositoryGoCachedModule(
	ctx context.Context,
	viewRoot string,
	hostCache string,
	item repositoryGoResolvedModule,
	prior repositoryGoModuleViewUsage,
	limits repositoryGoModuleViewLimits,
) (repositoryGoModuleViewUsage, error) {
	escapedPath, err := module.EscapePath(item.Path)
	if err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("escape repository build-list module path: %w", err)
	}
	escapedVersion, err := module.EscapeVersion(item.Version)
	if err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("escape repository build-list module version: %w", err)
	}
	relativeDownload := filepath.Join("cache", "download", filepath.FromSlash(escapedPath), "@v")
	sourceDownload := filepath.Join(hostCache, relativeDownload)
	targetDownload := filepath.Join(viewRoot, relativeDownload)
	if err := os.MkdirAll(targetDownload, 0o700); err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("create projected module metadata directory: %w", err)
	}
	type cacheFile struct {
		suffix string
		limit  int64
	}
	files := []cacheFile{
		{suffix: ".mod", limit: modulezip.MaxGoMod},
		{suffix: ".info", limit: 64 * 1024},
		{suffix: ".zip", limit: modulezip.MaxZipFile},
		{suffix: ".ziphash", limit: 256},
	}
	var copiedTotal int64
	for _, file := range files {
		name := escapedVersion + file.suffix
		remaining := limits.MaxRegularBytes - prior.RegularBytes - copiedTotal
		copyLimit := file.limit
		if remaining < copyLimit {
			copyLimit = remaining
		}
		copied, copyErr := copyExactRepositoryModuleCacheFile(
			ctx, filepath.Join(sourceDownload, name), filepath.Join(targetDownload, name),
			copyLimit, item.Path+"@"+item.Version+file.suffix,
		)
		if copyErr != nil {
			return repositoryGoModuleViewUsage{}, copyErr
		}
		copiedTotal += copied
	}
	zipPath := filepath.Join(targetDownload, escapedVersion+".zip")
	zipSum, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("hash exact cached module zip %s@%s: %w", item.Path, item.Version, err)
	}
	zipHashRaw, err := os.ReadFile(filepath.Join(targetDownload, escapedVersion+".ziphash"))
	if err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("read exact cached module zip hash: %w", err)
	}
	if zipSum != item.Sum || strings.TrimSpace(string(zipHashRaw)) != item.Sum {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("exact cached module zip authority differs for %s@%s", item.Path, item.Version)
	}
	modPath := filepath.Join(targetDownload, escapedVersion+".mod")
	modSum, err := repositoryGoModFileSum(item.Path, item.Version, modPath)
	if err != nil {
		return repositoryGoModuleViewUsage{}, err
	}
	if modSum != item.GoModSum {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("exact cached module go.mod authority differs for %s@%s", item.Path, item.Version)
	}
	if err := validateRepositoryGoModuleInfo(
		filepath.Join(targetDownload, escapedVersion+".info"), item.Version,
	); err != nil {
		return repositoryGoModuleViewUsage{}, err
	}
	actual, err := inspectRepositoryGoCachedModuleUsage(
		ctx, viewRoot, item, limits.MaxEntries-prior.Entries,
	)
	if err != nil {
		return repositoryGoModuleViewUsage{}, err
	}
	combined, err := prior.add(actual, limits)
	if err != nil {
		return repositoryGoModuleViewUsage{}, err
	}
	destination := filepath.Join(viewRoot, filepath.FromSlash(escapedPath)+"@"+escapedVersion)
	if err := modulezip.Unzip(destination, module.Version{
		Path: item.Path, Version: item.Version,
	}, zipPath); err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("extract exact cached module %s@%s: %w", item.Path, item.Version, err)
	}
	return combined, nil
}

func copyExactRepositoryModuleCacheFile(
	ctx context.Context,
	source string,
	destination string,
	maxBytes int64,
	label string,
) (int64, error) {
	before, err := inspectRepositoryGoModuleCacheFile(source, maxBytes, label)
	if err != nil {
		return 0, err
	}
	input, err := os.Open(source)
	if err != nil {
		return 0, fmt.Errorf("open exact cached module %s: %w", label, err)
	}
	defer input.Close()
	opened, err := input.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return 0, fmt.Errorf("exact cached module %s changed while it was opened", label)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o444)
	if err != nil {
		return 0, fmt.Errorf("create projected module %s: %w", label, err)
	}
	written, copyErr := io.Copy(output, &repositoryModuleContextReader{ctx: ctx, reader: input})
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || written != before.Size() {
		return 0, fmt.Errorf("copy exact cached module %s: %v", label, errorsJoinText(copyErr, closeErr))
	}
	after, err := os.Lstat(source)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() ||
		before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return 0, fmt.Errorf("exact cached module %s changed while it was copied", label)
	}
	return written, nil
}

type repositoryModuleContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *repositoryModuleContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func repositoryGoModFileSum(path, version, file string) (string, error) {
	name := "go.mod"
	sum, err := dirhash.Hash1([]string{name}, func(requested string) (io.ReadCloser, error) {
		if requested != name {
			return nil, os.ErrNotExist
		}
		return os.Open(file)
	})
	if err != nil {
		return "", fmt.Errorf("hash exact cached module go.mod %s@%s: %w", path, version, err)
	}
	return sum, nil
}

func validateRepositoryGoModuleInfo(path, version string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read exact cached module info: %w", err)
	}
	var value struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal(raw, &value); err != nil || value.Version != version {
		return fmt.Errorf("exact cached module info differs from version %q", version)
	}
	return nil
}

func errorsJoinText(values ...error) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value != nil {
			parts = append(parts, value.Error())
		}
	}
	if len(parts) == 0 {
		return "incomplete copy"
	}
	return strings.Join(parts, "; ")
}
