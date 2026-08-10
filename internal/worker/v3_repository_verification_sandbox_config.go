package worker

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	repositoryBubblewrapPath = "/usr/bin/bwrap"
	repositorySandboxRoot    = "/workspace"
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
	return nil
}

func existingRepositoryGoModuleCache() (string, error) {
	candidate := strings.TrimSpace(os.Getenv("GOMODCACHE"))
	if candidate == "" {
		goPath := strings.TrimSpace(build.Default.GOPATH)
		if goPath == "" {
			return "", fmt.Errorf("existing-repository verification cannot resolve one Go module cache")
		}
		candidate = filepath.Join(filepath.SplitList(goPath)[0], "pkg", "mod")
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
	rootFD int,
	goRootFD int,
	moduleCacheFD int,
	infoFD int,
	goArgs []string,
) []string {
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
		"--symlink", "usr/bin", "/bin",
		"--symlink", "usr/lib", "/lib",
		"--symlink", "usr/lib", "/lib64",
		"--symlink", "usr/sbin", "/sbin",
		"--proc", "/proc", "--dev", "/dev",
		"--tmpfs", "/tmp", "--tmpfs", "/home",
		"--dir", "/home/omnidex", "--dir", repositorySandboxRoot,
		"--dir", "/toolchain", "--dir", "/gomodcache",
		"--ro-bind-fd", fmt.Sprint(rootFD), repositorySandboxRoot,
		"--ro-bind-fd", fmt.Sprint(goRootFD), "/toolchain",
		"--ro-bind-fd", fmt.Sprint(moduleCacheFD), "/gomodcache",
	}
	arguments = append(arguments,
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
		"--info-fd", fmt.Sprint(infoFD), "--", "/toolchain/bin/go",
	)
	return append(arguments, goArgs...)
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
