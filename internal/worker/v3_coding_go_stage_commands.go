package worker

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func directCodingGoVerificationCommand(
	sourceRoot string,
	cacheRoot string,
	moduleCacheRoot string,
	arguments ...string,
) (directCodingVerificationCommand, error) {
	environment, err := directCodingGoCommandEnvironment(
		sourceRoot, cacheRoot, moduleCacheRoot,
	)
	if err != nil {
		return directCodingVerificationCommand{}, err
	}
	if len(arguments) == 0 {
		return directCodingVerificationCommand{}, fmt.Errorf("Go verification command requires arguments")
	}
	for _, argument := range arguments {
		if argument == "" || strings.ContainsRune(argument, 0) {
			return directCodingVerificationCommand{}, fmt.Errorf("Go verification command contains an invalid argument")
		}
	}
	return directCodingVerificationCommand{
		Argv:        append([]string{"go"}, arguments...),
		Environment: environment,
		Timeout:     defaultDirectCodingVerificationTimeout,
	}, nil
}

func directCodingGoFormatCheckCommand(
	sourceRoot string,
	cacheRoot string,
	moduleCacheRoot string,
	sourcePaths ...string,
) (directCodingVerificationCommand, error) {
	environment, err := directCodingGoCommandEnvironment(
		sourceRoot, cacheRoot, moduleCacheRoot,
	)
	if err != nil {
		return directCodingVerificationCommand{}, err
	}
	paths, err := directCodingCanonicalGoSourcePaths(sourcePaths)
	if err != nil {
		return directCodingVerificationCommand{}, err
	}
	return directCodingVerificationCommand{
		Argv:        append([]string{"gofmt", "-d", "--"}, paths...),
		Environment: environment,
		Timeout:     defaultDirectCodingVerificationTimeout,
	}, nil
}

func validateDirectCodingGoFormatCheck(result directCodingVerificationCommandResult) error {
	if len(result.Stdout) != 0 {
		return fmt.Errorf("gofmt reported source that is not canonically formatted")
	}
	if len(result.Stderr) != 0 {
		return fmt.Errorf("gofmt emitted unexpected stderr: %s", strings.TrimSpace(string(result.Stderr)))
	}
	return nil
}

func directCodingGoCommandEnvironment(
	sourceRoot string,
	cacheRoot string,
	moduleCacheRoot string,
) ([]string, error) {
	if err := directCodingRequireExternalGoCache(sourceRoot, cacheRoot, "build"); err != nil {
		return nil, err
	}
	if err := directCodingRequireExternalGoCache(sourceRoot, moduleCacheRoot, "module"); err != nil {
		return nil, err
	}
	if cacheRoot == moduleCacheRoot {
		return nil, fmt.Errorf("Go build and module cache roots must be distinct")
	}
	environment := []string{
		"GOCACHE=" + cacheRoot,
		"GOENV=off",
		"GOFLAGS=-buildvcs=false -mod=readonly",
		"GOMODCACHE=" + moduleCacheRoot,
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOTELEMETRY=off",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	}
	sort.Strings(environment)
	return environment, nil
}

func directCodingRequireExternalGoCache(
	sourceRoot string,
	cacheRoot string,
	kind string,
) error {
	if !filepath.IsAbs(sourceRoot) || filepath.Clean(sourceRoot) != sourceRoot {
		return fmt.Errorf("Go command requires one canonical absolute source root")
	}
	if !filepath.IsAbs(cacheRoot) || filepath.Clean(cacheRoot) != cacheRoot {
		return fmt.Errorf("Go %s cache requires one canonical absolute root", kind)
	}
	relative, err := filepath.Rel(sourceRoot, cacheRoot)
	if err != nil {
		return fmt.Errorf("compare Go %s cache with source root: %w", kind, err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return fmt.Errorf("Go %s cache must be outside the source root", kind)
	}
	return nil
}

func directCodingCanonicalGoSourcePaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("gofmt check requires at least one Go source path")
	}
	canonical := append([]string(nil), paths...)
	for _, path := range canonical {
		if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path ||
			path == "." || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) ||
			filepath.Ext(path) != ".go" || strings.ContainsRune(path, 0) {
			return nil, fmt.Errorf("gofmt check contains an invalid source path")
		}
	}
	sort.Strings(canonical)
	for index := 1; index < len(canonical); index++ {
		if canonical[index] == canonical[index-1] {
			return nil, fmt.Errorf("gofmt check repeats source path %s", canonical[index])
		}
	}
	return canonical, nil
}
