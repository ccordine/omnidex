package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/module"
)

const (
	maxRepositoryGoResolvedModules      = 512
	maxRepositoryGoBuildListBytes       = 4 * 1024 * 1024
	maxRepositoryGoBuildListStderrBytes = 64 * 1024
)

type repositoryGoResolvedModule struct {
	Path     string
	Version  string
	Sum      string
	GoModSum string
}

type repositoryGoListModule struct {
	Path     string                  `json:"Path"`
	Version  string                  `json:"Version"`
	Main     bool                    `json:"Main"`
	Dir      string                  `json:"Dir"`
	Sum      string                  `json:"Sum"`
	GoModSum string                  `json:"GoModSum"`
	Replace  *repositoryGoListModule `json:"Replace"`
}

func resolveRepositoryGoBuildList(
	ctx context.Context,
	root string,
	config repositoryGoSandboxConfig,
) (modules []repositoryGoResolvedModule, resultErr error) {
	if ctx == nil {
		return nil, fmt.Errorf("resolve repository Go build list requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("resolve repository Go build list: %w", err)
	}
	if _, err := exactRepositorySandboxDirectory(root, "repository module source"); err != nil {
		return nil, err
	}
	if err := validateRepositoryGoModuleSource(config); err != nil {
		return nil, err
	}
	if err := exactRepositoryRegularFile(filepath.Join(root, "go.mod"), "repository go.mod"); err != nil {
		return nil, err
	}
	home, err := os.MkdirTemp("", "omnidex-repository-go-list-*")
	if err != nil {
		return nil, fmt.Errorf("create repository Go build-list environment: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(home); cleanupErr != nil {
			modules = nil
			resultErr = errors.Join(resultErr, fmt.Errorf("clean repository Go build-list environment: %w", cleanupErr))
		}
	}()
	if err := prepareRepositoryGoBuildListHome(home); err != nil {
		return nil, err
	}
	command := exec.CommandContext(
		ctx, filepath.Join(config.GoRoot, "bin", "go"),
		"list", "-mod=readonly", "-m", "-json", "all",
	)
	command.Dir = root
	command.Env = repositoryGoBuildListEnvironment(config, home)
	stdout := newExactRepositoryCommandOutput(maxRepositoryGoBuildListBytes)
	stderr := newExactRepositoryCommandOutput(maxRepositoryGoBuildListStderrBytes)
	command.Stdout = stdout
	command.Stderr = stderr
	err = command.Run()
	outputErr := errors.Join(
		stdout.Validate("repository Go build-list stdout"),
		stderr.Validate("repository Go build-list stderr"),
	)
	if err != nil && ctx.Err() != nil {
		return nil, errors.Join(
			fmt.Errorf("resolve repository Go build list: %w", ctx.Err()),
			outputErr,
		)
	}
	if outputErr != nil {
		return nil, outputErr
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf(
				"resolve exact offline repository Go build list: %s",
				trimForBudget(strings.TrimSpace(stderr.String()), 2048),
			)
		}
		return nil, fmt.Errorf("execute exact offline repository Go build list: %w", err)
	}
	raw := []byte(stdout.String())
	if len(raw) == 0 {
		return nil, fmt.Errorf("repository Go build list is empty")
	}
	return decodeRepositoryGoBuildList(root, raw)
}

func prepareRepositoryGoBuildListHome(home string) error {
	telemetryDirectory := filepath.Join(home, ".config", "go", "telemetry")
	if err := os.MkdirAll(telemetryDirectory, 0o700); err != nil {
		return fmt.Errorf("create repository Go build-list telemetry boundary: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(telemetryDirectory, "mode"), []byte("off 2000-01-01"), 0o600,
	); err != nil {
		return fmt.Errorf("disable repository Go build-list telemetry: %w", err)
	}
	return nil
}

func validateRepositoryGoModuleSource(config repositoryGoSandboxConfig) error {
	if _, err := exactRepositorySandboxDirectory(config.GoRoot, "system Go toolchain"); err != nil {
		return err
	}
	if err := exactRepositorySandboxExecutable(
		filepath.Join(config.GoRoot, "bin", "go"), "system Go executable",
	); err != nil {
		return err
	}
	if _, err := exactRepositorySandboxDirectory(config.ModuleCache, "host Go module cache source"); err != nil {
		return err
	}
	return nil
}

func repositoryGoBuildListEnvironment(config repositoryGoSandboxConfig, home string) []string {
	return []string{
		"HOME=" + home,
		"PATH=" + filepath.Join(config.GoRoot, "bin") + ":/usr/bin:/bin",
		"GOROOT=" + config.GoRoot,
		"GOPATH=" + filepath.Join(home, "gopath"),
		"GOCACHE=" + filepath.Join(home, "gocache"),
		"GOMODCACHE=" + config.ModuleCache,
		"GO111MODULE=on", "GOENV=off", "GOWORK=off", "GOTOOLCHAIN=local",
		"GOPROXY=off", "GOSUMDB=off", "GOVCS=off",
		"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "TZ=UTC",
	}
}

func decodeRepositoryGoBuildList(root string, raw []byte) ([]repositoryGoResolvedModule, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	modules := make([]repositoryGoResolvedModule, 0)
	seen := make(map[string]struct{})
	mainModules := 0
	for {
		var item repositoryGoListModule
		if err := decoder.Decode(&item); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode repository Go build list: %w", err)
		}
		if item.Main {
			mainModules++
			continue
		}
		effective := item
		if item.Replace != nil {
			effective = *item.Replace
			if effective.Version == "" {
				if err := requireRepositoryLocalModuleWithinRoot(root, effective.Dir); err != nil {
					return nil, err
				}
				continue
			}
		}
		if _, err := module.EscapePath(effective.Path); err != nil {
			return nil, fmt.Errorf("repository build-list module path is invalid: %w", err)
		}
		if _, err := module.EscapeVersion(effective.Version); err != nil {
			return nil, fmt.Errorf("repository build-list module version is invalid: %w", err)
		}
		resolved := repositoryGoResolvedModule{
			Path: effective.Path, Version: effective.Version,
			Sum: effective.Sum, GoModSum: effective.GoModSum,
		}
		if !validRepositoryModuleSum(resolved.Sum) || !validRepositoryModuleSum(resolved.GoModSum) {
			return nil, fmt.Errorf(
				"repository build-list module %s@%s lacks exact cached checksum authority",
				resolved.Path, resolved.Version,
			)
		}
		key := resolved.Path + "\x00" + resolved.Version
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		modules = append(modules, resolved)
		if len(modules) > maxRepositoryGoResolvedModules {
			return nil, fmt.Errorf(
				"repository Go build list exceeds %d exact dependency modules",
				maxRepositoryGoResolvedModules,
			)
		}
	}
	if mainModules != 1 {
		return nil, fmt.Errorf("repository Go build list contains %d main modules; require exactly one", mainModules)
	}
	sort.Slice(modules, func(left, right int) bool {
		if modules[left].Path == modules[right].Path {
			return modules[left].Version < modules[right].Version
		}
		return modules[left].Path < modules[right].Path
	})
	return modules, nil
}

func validRepositoryModuleSum(value string) bool {
	return strings.HasPrefix(value, "h1:") && len(value) > len("h1:") &&
		value == strings.TrimSpace(value)
}

func requireRepositoryLocalModuleWithinRoot(root, directory string) error {
	if directory == "" || !filepath.IsAbs(directory) {
		return fmt.Errorf("repository local module replacement has no exact absolute directory")
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("repository local module replacement escapes the snapshot-only workspace")
	}
	if _, err := exactRepositorySandboxDirectory(directory, "repository local module replacement"); err != nil {
		return err
	}
	return nil
}

func exactRepositoryRegularFile(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is absent or not one exact regular file", label)
	}
	return nil
}
