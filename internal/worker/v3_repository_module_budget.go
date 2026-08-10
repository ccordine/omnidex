package worker

import (
	"archive/zip"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
	modulezip "golang.org/x/mod/zip"
)

const (
	maxRepositoryGoModuleViewBytes   int64 = 4 << 30
	maxRepositoryGoModuleViewEntries       = 500_000
)

type repositoryGoModuleViewLimits struct {
	MaxRegularBytes int64
	MaxEntries      int
}

type repositoryGoModuleViewUsage struct {
	RegularBytes int64
	Entries      int
}

func defaultRepositoryGoModuleViewLimits() repositoryGoModuleViewLimits {
	return repositoryGoModuleViewLimits{
		MaxRegularBytes: maxRepositoryGoModuleViewBytes,
		MaxEntries:      maxRepositoryGoModuleViewEntries,
	}
}

func (limits repositoryGoModuleViewLimits) Validate() error {
	if limits.MaxRegularBytes < 1 || limits.MaxRegularBytes > maxRepositoryGoModuleViewBytes {
		return fmt.Errorf(
			"repository Go module view regular-byte limit must be between 1 and %d",
			maxRepositoryGoModuleViewBytes,
		)
	}
	if limits.MaxEntries < 1 || limits.MaxEntries > maxRepositoryGoModuleViewEntries {
		return fmt.Errorf(
			"repository Go module view entry limit must be between 1 and %d",
			maxRepositoryGoModuleViewEntries,
		)
	}
	return nil
}

func (usage repositoryGoModuleViewUsage) add(
	addition repositoryGoModuleViewUsage,
	limits repositoryGoModuleViewLimits,
) (repositoryGoModuleViewUsage, error) {
	if addition.RegularBytes < 0 || usage.RegularBytes > limits.MaxRegularBytes-addition.RegularBytes {
		return repositoryGoModuleViewUsage{}, fmt.Errorf(
			"repository Go module view exceeds exact %d-regular-byte limit",
			limits.MaxRegularBytes,
		)
	}
	if addition.Entries < 0 || usage.Entries > limits.MaxEntries-addition.Entries {
		return repositoryGoModuleViewUsage{}, fmt.Errorf(
			"repository Go module view exceeds exact %d-entry limit",
			limits.MaxEntries,
		)
	}
	return repositoryGoModuleViewUsage{
		RegularBytes: usage.RegularBytes + addition.RegularBytes,
		Entries:      usage.Entries + addition.Entries,
	}, nil
}

func inspectRepositoryGoCachedModuleUsage(
	ctx context.Context,
	cacheRoot string,
	item repositoryGoResolvedModule,
	maxEntries int,
) (repositoryGoModuleViewUsage, error) {
	if err := ctx.Err(); err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("inspect repository module-view budget: %w", err)
	}
	escapedPath, err := module.EscapePath(item.Path)
	if err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("escape repository build-list module path: %w", err)
	}
	escapedVersion, err := module.EscapeVersion(item.Version)
	if err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("escape repository build-list module version: %w", err)
	}
	downloadRelative := filepath.ToSlash(filepath.Join(
		"cache", "download", filepath.FromSlash(escapedPath), "@v",
	))
	download := filepath.Join(cacheRoot, filepath.FromSlash(downloadRelative))
	usage := repositoryGoModuleViewUsage{}
	for suffix, limit := range map[string]int64{
		".mod": modulezip.MaxGoMod, ".info": 64 * 1024,
		".zip": modulezip.MaxZipFile, ".ziphash": 256,
	} {
		relative := downloadRelative + "/" + escapedVersion + suffix
		info, inspectErr := inspectRepositoryGoModuleCacheFile(
			filepath.Join(download, escapedVersion+suffix), limit,
			item.Path+"@"+item.Version+suffix,
		)
		if inspectErr != nil {
			return repositoryGoModuleViewUsage{}, inspectErr
		}
		usage, err = usage.addUnbounded(info.Size(), conservativeRepositoryPathEntries(relative))
		if err != nil {
			return repositoryGoModuleViewUsage{}, err
		}
	}
	zipPath := filepath.Join(download, escapedVersion+".zip")
	remainingEntries := maxEntries - usage.Entries
	if remainingEntries < 1 {
		return repositoryGoModuleViewUsage{}, fmt.Errorf(
			"repository module ZIP exceeds remaining exact %d-entry limit", maxEntries,
		)
	}
	if _, err := preflightRepositoryGoModuleZip(zipPath, remainingEntries); err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf(
			"preflight exact cached module archive %s@%s: %w", item.Path, item.Version, err,
		)
	}
	if _, err := modulezip.CheckZip(module.Version{Path: item.Path, Version: item.Version}, zipPath); err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf(
			"inspect exact cached module archive %s@%s: %w", item.Path, item.Version, err,
		)
	}
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return repositoryGoModuleViewUsage{}, fmt.Errorf(
			"open exact cached module archive %s@%s: %w", item.Path, item.Version, err,
		)
	}
	defer archive.Close()
	prefix := item.Path + "@" + item.Version + "/"
	extractedRoot := escapedPath + "@" + escapedVersion
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return repositoryGoModuleViewUsage{}, fmt.Errorf("inspect repository module-view budget: %w", err)
		}
		name := strings.TrimPrefix(entry.Name, prefix)
		if name == "" {
			continue
		}
		if entry.UncompressedSize64 > math.MaxInt64 {
			return repositoryGoModuleViewUsage{}, fmt.Errorf("repository module archive size overflows exact budget")
		}
		regularBytes := int64(entry.UncompressedSize64)
		if strings.HasSuffix(name, "/") {
			regularBytes = 0
		}
		usage, err = usage.addUnbounded(
			regularBytes,
			conservativeRepositoryPathEntries(extractedRoot+"/"+strings.TrimSuffix(name, "/")),
		)
		if err != nil {
			return repositoryGoModuleViewUsage{}, err
		}
	}
	return usage, nil
}

func inspectRepositoryGoModuleCacheFile(path string, maxBytes int64, label string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() < 1 || info.Size() > maxBytes {
		return nil, fmt.Errorf(
			"exact cached module %s is absent, unsafe, empty, or exceeds %d bytes", label, maxBytes,
		)
	}
	return info, nil
}

func conservativeRepositoryPathEntries(path string) int {
	cleaned := strings.Trim(filepath.ToSlash(path), "/")
	if cleaned == "" {
		return 0
	}
	return 1 + strings.Count(cleaned, "/")
}

func (usage repositoryGoModuleViewUsage) addUnbounded(
	regularBytes int64,
	entries int,
) (repositoryGoModuleViewUsage, error) {
	if regularBytes < 0 || entries < 0 || usage.RegularBytes > math.MaxInt64-regularBytes ||
		usage.Entries > math.MaxInt-entries {
		return repositoryGoModuleViewUsage{}, fmt.Errorf("repository Go module view budget overflows")
	}
	usage.RegularBytes += regularBytes
	usage.Entries += entries
	return usage, nil
}
