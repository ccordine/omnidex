package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryGoSandboxArgumentsHaveReadOnlyRepositoryAndNoNetwork(t *testing.T) {
	t.Parallel()
	_, projection, _, _ := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 0))
	arguments, err := repositoryGoSandboxArguments(
		projection, repositoryWorkspaceProjectionMountRoots{base: projection.source.Root},
		3, -1, 4, 5, 6,
	)
	if err != nil {
		t.Fatal(err)
	}
	joined := " " + strings.Join(arguments, " ") + " "
	for _, required := range []string{
		" --unshare-all ", " --unshare-user ", " --disable-userns ", " --clearenv ",
		" --ro-bind /proc/self/fd/3/go.mod /workspace/go.mod ", " --ro-bind-fd 4 /toolchain ",
		" --ro-bind-fd 5 /gomodcache ",
		" --tmpfs /workspace ", " --remount-ro /workspace ",
		" --tmpfs /tmp ", " --tmpfs /home ", " --chdir /workspace ",
		" --setenv HOME /home/omnidex ", " --setenv GOCACHE /tmp/gocache ",
		" --setenv TMPDIR /tmp ", " --setenv GO111MODULE on ",
		" --setenv GOFLAGS -mod=readonly ", " --setenv GOWORK off ",
		" --setenv GOPROXY off ",
		" --setenv GOSUMDB off ", " --setenv GOVCS off ",
		" --ro-bind /bin /bin ", " --ro-bind /lib /lib ",
		" --ro-bind /sbin /sbin ", " --ro-bind /usr/sbin /usr/sbin ",
		" --ro-bind-try /lib64 /lib64 ", " --ro-bind-try /usr/lib64 /usr/lib64 ",
		" --info-fd 6 ",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("sandbox arguments omit %q:\n%s", required, joined)
		}
	}
	if strings.Contains(joined, " /toolchain/bin/go ") || strings.Contains(joined, " test ") {
		t.Fatalf("sandbox option authority contains the outer Go command: %s", joined)
	}
	for _, forbidden := range []string{
		" --share-net ", " --bind / ", " --ro-bind / / ", " --ro-bind /usr /usr ", " --ro-bind /home ",
		" /projection-base ", " /projection-delta ",
		" --setenv SSH_", " --setenv AWS_", " --setenv GITHUB_",
		" --symlink usr/bin /bin ", " --symlink usr/lib /lib ",
		" --symlink usr/lib /lib64 ", " --symlink usr/sbin /sbin ",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("sandbox arguments contain forbidden authority %q:\n%s", forbidden, joined)
		}
	}
}

func TestResolvableRepositorySandboxDirectoryAcceptsDirectoriesAndDirectorySymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "directory-link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{directory, link} {
		if err := resolvableRepositorySandboxDirectory(path, "fixture runtime path"); err != nil {
			t.Fatalf("path %q: %v", path, err)
		}
	}
}

func TestResolvableRepositorySandboxDirectoryRejectsInvalidTargets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "missing-target"), dangling); err != nil {
		t.Fatal(err)
	}
	fileLink := filepath.Join(root, "file-link")
	if err := os.Symlink(file, fileLink); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"relative", filepath.Join(root, "missing"), dangling, file, fileLink,
	} {
		if err := resolvableRepositorySandboxDirectory(path, "fixture runtime path"); err == nil {
			t.Fatalf("invalid path %q was accepted", path)
		}
	}
}

func TestRepositoryGoSandboxNeverBindsHostModuleCache(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_repository_verification_sandbox.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "config.ModuleCache") {
		t.Fatal("repository test sandbox still binds the host-wide Go module cache")
	}
	if !strings.Contains(source, "moduleView.Root()") {
		t.Fatal("repository test sandbox does not bind one exact module view")
	}
}

func TestRepositoryGoSandboxConfigFailsWithoutBubblewrapOrExactToolchain(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	toolchain := filepath.Join(root, "toolchain")
	cache := filepath.Join(root, "module-cache")
	if err := os.MkdirAll(filepath.Join(toolchain, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolchain, "bin", "go"), []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := repositoryGoSandboxConfig{
		BubblewrapPath: filepath.Join(root, "missing-bwrap"),
		GoRoot:         toolchain,
		ModuleCache:    cache,
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "bubblewrap") {
		t.Fatalf("missing bubblewrap error=%v", err)
	}
	if err := os.WriteFile(config.BubblewrapPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "Go executable") {
		t.Fatalf("invalid Go toolchain error=%v", err)
	}
}

func TestExistingRepositoryGoModuleCacheRequiresExplicitCanonicalAuthority(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOMODCACHE", root)
	resolved, err := existingRepositoryGoModuleCache()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Fatalf("module cache=%q want %q", resolved, want)
	}

	for _, value := range []string{"", " " + root, "relative/cache", filepath.Join(root, "missing")} {
		t.Run(strings.ReplaceAll(value, "/", "_"), func(t *testing.T) {
			t.Setenv("GOMODCACHE", value)
			if _, err := existingRepositoryGoModuleCache(); err == nil {
				t.Fatalf("invalid GOMODCACHE %q was accepted", value)
			}
		})
	}
}

func TestRepositorySandboxRejectsAnyGitOrOmniWorkspaceAuthority(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".git", ".omni"} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := captureRepositoryVerificationTree(root); err == nil ||
			!strings.Contains(err.Error(), "snapshot-only") {
			t.Fatalf("forbidden %s authority error=%v", name, err)
		}
	}
}
