package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryGoSandboxArgumentsHaveReadOnlyRepositoryAndNoNetwork(t *testing.T) {
	t.Parallel()
	arguments := repositoryGoSandboxArguments(
		3, 4, 5, 6,
		[]string{"test", "./internal/sample"},
	)
	joined := " " + strings.Join(arguments, " ") + " "
	for _, required := range []string{
		" --unshare-all ", " --unshare-user ", " --disable-userns ", " --clearenv ",
		" --ro-bind-fd 3 /workspace ", " --ro-bind-fd 4 /toolchain ",
		" --ro-bind-fd 5 /gomodcache ",
		" --tmpfs /tmp ", " --tmpfs /home ", " --chdir /workspace ",
		" --setenv HOME /home/omnidex ", " --setenv GOCACHE /tmp/gocache ",
		" --setenv TMPDIR /tmp ", " --setenv GO111MODULE on ",
		" --setenv GOFLAGS -mod=readonly ", " --setenv GOWORK off ",
		" --setenv GOPROXY off ",
		" --setenv GOSUMDB off ", " --setenv GOVCS off ",
		" --info-fd 6 ", " -- /toolchain/bin/go test ./internal/sample ",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("sandbox arguments omit %q:\n%s", required, joined)
		}
	}
	for _, forbidden := range []string{
		" --share-net ", " --bind / ", " --ro-bind / / ", " --ro-bind /usr /usr ", " --ro-bind /home ",
		" --setenv SSH_", " --setenv AWS_", " --setenv GITHUB_",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("sandbox arguments contain forbidden authority %q:\n%s", forbidden, joined)
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
