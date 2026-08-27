package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	repositoryBubblewrapPath = "/usr/bin/bwrap"
	repositorySandboxRoot    = "/workspace"

	maxRepositoryWorkspaceProjectionArgumentsBytes = 32 * 1024 * 1024
	maxRepositoryBubblewrapParsedArguments         = 9_000
)

type repositoryGoSandboxConfig struct {
	BubblewrapPath string
	GoRoot         string
	ModuleCache    string
}

func discoverRepositoryGoSandbox() (repositoryGoSandboxConfig, error) {
	if runtime.GOOS != "linux" {
		return repositoryGoSandboxConfig{}, fmt.Errorf(
			"existing-repository Go verification requires Linux bubblewrap; host is %s",
			runtime.GOOS,
		)
	}
	goRoot, err := filepath.EvalSymlinks(runtime.GOROOT())
	if err != nil {
		return repositoryGoSandboxConfig{}, fmt.Errorf("resolve system Go toolchain: %w", err)
	}
	moduleCache, err := existingRepositoryGoModuleCache()
	if err != nil {
		return repositoryGoSandboxConfig{}, err
	}
	config := repositoryGoSandboxConfig{
		BubblewrapPath: repositoryBubblewrapPath,
		GoRoot:         goRoot,
		ModuleCache:    moduleCache,
	}
	if err := config.Validate(); err != nil {
		return repositoryGoSandboxConfig{}, err
	}
	return config, nil
}

func (config repositoryGoSandboxConfig) Validate() error {
	if err := config.validateExecution(); err != nil {
		return err
	}
	if _, err := exactRepositorySandboxDirectory(config.ModuleCache, "host Go module cache source"); err != nil {
		return err
	}
	return nil
}

func (config repositoryGoSandboxConfig) validateExecution() error {
	if err := exactRepositorySandboxExecutable(config.BubblewrapPath, "bubblewrap"); err != nil {
		return err
	}
	if _, err := exactRepositorySandboxDirectory(config.GoRoot, "system Go toolchain"); err != nil {
		return err
	}
	if err := exactRepositorySandboxExecutable(
		filepath.Join(config.GoRoot, "bin", "go"), "system Go executable",
	); err != nil {
		return err
	}
	for _, systemPath := range []string{
		"/usr/bin", "/usr/lib", "/usr/include", "/usr/libexec", "/usr/share",
	} {
		if _, err := exactRepositorySandboxDirectory(systemPath, "read-only system tool path"); err != nil {
			return err
		}
	}
	for _, systemPath := range []string{
		"/bin", "/lib", "/sbin", "/usr/sbin",
	} {
		if err := resolvableRepositorySandboxDirectory(systemPath, "read-only system runtime path"); err != nil {
			return err
		}
	}
	return nil
}

func existingRepositoryGoModuleCache() (string, error) {
	raw, configured := os.LookupEnv("GOMODCACHE")
	candidate := strings.TrimSpace(raw)
	if !configured || candidate == "" {
		return "", fmt.Errorf("existing-repository verification requires explicit GOMODCACHE authority")
	}
	if candidate != raw {
		return "", fmt.Errorf("existing Go module cache must be canonical exact text")
	}
	if !filepath.IsAbs(candidate) {
		return "", fmt.Errorf("existing Go module cache must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve existing Go module cache: %w", err)
	}
	return resolved, nil
}

func repositoryGoSandboxArguments(
	projection repositoryWorkspaceProjection,
	mountRoots repositoryWorkspaceProjectionMountRoots,
	baseFD int,
	deltaFD int,
	goRootFD int,
	moduleCacheFD int,
	infoFD int,
) ([]string, error) {
	if err := projection.validate(); err != nil {
		return nil, err
	}
	if baseFD < 3 {
		return nil, fmt.Errorf("repository projection requires one inherited source descriptor")
	}
	arguments := []string{
		"--unshare-all", "--unshare-user", "--disable-userns", "--assert-userns-disabled",
		"--die-with-parent", "--new-session", "--cap-drop", "ALL",
		"--hostname", "omnidex-verification", "--clearenv",
		"--dir", "/usr",
		"--ro-bind", "/usr/bin", "/usr/bin",
		"--ro-bind", "/usr/lib", "/usr/lib",
		"--ro-bind", "/usr/include", "/usr/include",
		"--ro-bind", "/usr/libexec", "/usr/libexec",
		"--ro-bind", "/usr/share", "/usr/share",
		"--ro-bind", "/usr/sbin", "/usr/sbin",
		"--ro-bind", "/bin", "/bin",
		"--ro-bind", "/lib", "/lib",
		"--ro-bind", "/sbin", "/sbin",
		"--ro-bind-try", "/lib64", "/lib64",
		"--ro-bind-try", "/usr/lib64", "/usr/lib64",
	}
	if projection.deltaRoot != "" {
		if deltaFD < 3 {
			return nil, fmt.Errorf("staged repository projection requires one inherited delta descriptor")
		}
	} else if deltaFD != -1 {
		return nil, fmt.Errorf("snapshot repository projection received an unexpected delta descriptor")
	}
	arguments = append(arguments,
		"--tmpfs", repositorySandboxRoot,
	)
	mounts, err := repositoryWorkspaceProjectionMounts(projection, mountRoots)
	if err != nil {
		return nil, err
	}
	for _, mount := range mounts {
		destination := repositorySandboxPath(mount.Path)
		switch mount.Source {
		case repositoryWorkspaceProjectionBase:
			arguments = append(arguments,
				"--ro-bind", repositorySandboxDescriptorPath(baseFD, mount.Path), destination,
			)
		case repositoryWorkspaceProjectionDelta:
			arguments = append(arguments,
				"--ro-bind", repositorySandboxDescriptorPath(deltaFD, mount.Path), destination,
			)
		case repositoryWorkspaceProjectionSymlink:
			if mount.Directory {
				return nil, fmt.Errorf("repository symlink projection cannot bind directory %q", mount.Path)
			}
			arguments = append(arguments, "--symlink", mount.LinkTarget, destination)
		default:
			return nil, fmt.Errorf("repository workspace projection mount %q has unsupported source %q", mount.Path, mount.Source)
		}
	}
	arguments = append(arguments,
		"--remount-ro", repositorySandboxRoot,
		"--proc", "/proc", "--dev", "/dev",
		"--tmpfs", "/tmp", "--tmpfs", "/home",
		"--dir", "/home/omnidex",
		"--dir", "/toolchain", "--dir", "/gomodcache",
		"--ro-bind-fd", fmt.Sprint(goRootFD), "/toolchain",
		"--ro-bind-fd", fmt.Sprint(moduleCacheFD), "/gomodcache",
		"--chdir", repositorySandboxRoot,
		"--setenv", "HOME", "/home/omnidex",
		"--setenv", "PATH", "/toolchain/bin:/usr/bin:/bin",
		"--setenv", "PWD", repositorySandboxRoot,
		"--setenv", "GOCACHE", "/tmp/gocache",
		"--setenv", "GOMODCACHE", "/gomodcache",
		"--setenv", "GOPATH", "/home/omnidex/go",
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "GO111MODULE", "on",
		"--setenv", "GOENV", "off",
		"--setenv", "GOFLAGS", "-mod=readonly",
		"--setenv", "GOWORK", "off",
		"--setenv", "GOTOOLCHAIN", "local",
		"--setenv", "GOPROXY", "off",
		"--setenv", "GOSUMDB", "off",
		"--setenv", "GOVCS", "off",
		"--setenv", "LANG", "C.UTF-8",
		"--setenv", "LC_ALL", "C.UTF-8",
		"--setenv", "TZ", "UTC",
		"--info-fd", fmt.Sprint(infoFD),
	)
	if err := validateRepositoryWorkspaceProjectionArguments(arguments); err != nil {
		return nil, err
	}
	return arguments, nil
}

func repositorySandboxPath(relative string) string {
	return filepath.Join(repositorySandboxRoot, filepath.FromSlash(relative))
}

func repositorySandboxDescriptorPath(descriptor int, relative string) string {
	return filepath.Join("/proc/self/fd", fmt.Sprint(descriptor), filepath.FromSlash(relative))
}

func validateRepositoryWorkspaceProjectionArguments(arguments []string) error {
	if len(arguments) > maxRepositoryBubblewrapParsedArguments {
		return fmt.Errorf(
			"repository workspace projection requires %d Bubblewrap arguments; hard limit is %d",
			len(arguments), maxRepositoryBubblewrapParsedArguments,
		)
	}
	total := 0
	for _, argument := range arguments {
		if strings.ContainsRune(argument, '\x00') {
			return fmt.Errorf("repository workspace projection argument contains NUL authority")
		}
		if total > maxRepositoryWorkspaceProjectionArgumentsBytes-len(argument)-1 {
			return fmt.Errorf(
				"repository workspace projection arguments exceed %d bytes",
				maxRepositoryWorkspaceProjectionArgumentsBytes,
			)
		}
		total += len(argument) + 1
	}
	return nil
}

func repositoryBubblewrapInvocation(
	options []string,
	argumentsFD int,
	goArgs []string,
) ([]string, error) {
	if argumentsFD < 3 {
		return nil, fmt.Errorf("repository verification requires one inherited Bubblewrap argument descriptor")
	}
	invocation := append(
		[]string{"--args", fmt.Sprint(argumentsFD), "--", "/toolchain/bin/go"},
		goArgs...,
	)
	if len(options)+len(invocation) > maxRepositoryBubblewrapParsedArguments {
		return nil, fmt.Errorf(
			"repository verification requires %d total Bubblewrap arguments; hard limit is %d",
			len(options)+len(invocation), maxRepositoryBubblewrapParsedArguments,
		)
	}
	if err := validateRepositoryWorkspaceProjectionArguments(invocation); err != nil {
		return nil, err
	}
	return invocation, nil
}

func openRepositorySandboxDirectory(path, label string) (*os.File, error) {
	before, err := exactRepositorySandboxDirectory(path, label)
	if err != nil {
		return nil, err
	}
	handle, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", label, err)
	}
	after, err := handle.Stat()
	if err != nil || !after.IsDir() || !os.SameFile(before, after) {
		_ = handle.Close()
		return nil, fmt.Errorf("%s changed while it was opened", label)
	}
	return handle, nil
}

func exactRepositorySandboxDirectory(path, label string) (os.FileInfo, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%s must be one absolute directory", label)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is absent or not one exact directory", label)
	}
	return info, nil
}

func resolvableRepositorySandboxDirectory(path, label string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("%s must be one absolute canonical directory", label)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("%s is absent or does not resolve to one directory", label)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%s is absent or does not resolve to one directory", label)
	}
	return nil
}

func exactRepositorySandboxExecutable(path, label string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("%s must be one absolute executable", label)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s is absent or not one exact executable", label)
	}
	return nil
}
