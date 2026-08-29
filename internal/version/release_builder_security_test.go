package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseBuilderRejectsUnsafeCoordinatesBeforeBuild(t *testing.T) {
	repository := releaseRepositoryRoot(t)
	script := filepath.Join(repository, "scripts", "build-release.sh")
	safeDist := filepath.Join(repository, ".agent-cache", "release-builder-security")
	symlinkTarget := filepath.Join(repository, ".agent-cache", "release-builder-security-real")
	symlinkDist := filepath.Join(repository, ".agent-cache", "release-builder-security-link")
	if err := os.MkdirAll(symlinkTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(symlinkTarget, symlinkDist); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(safeDist)
		_ = os.Remove(symlinkDist)
		_ = os.RemoveAll(symlinkTarget)
	})
	outsideDist, err := os.MkdirTemp("/tmp", "omnidex-release-builder-outside-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outsideDist) })

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "version traversal", args: []string{"--dist", safeDist, "--version", "../../escape"}, want: "invalid release version"},
		{name: "version length", args: []string{"--dist", safeDist, "--version", "v1.2.3-" + strings.Repeat("a", 65)}, want: "exceeds 64 characters"},
		{name: "codename traversal", args: []string{"--dist", safeDist, "--codename", "../escape"}, want: "invalid release codename"},
		{name: "target traversal", args: []string{"--dist", safeDist, "--target", "linux/../../escape"}, want: "unsupported release target"},
		{name: "outside repository", args: []string{"--dist", outsideDist, "--target", "linux/amd64"}, want: "strict repository descendant"},
		{name: "repository root", args: []string{"--dist", repository, "--target", "linux/amd64"}, want: "strict repository descendant"},
		{name: "tracked internal", args: []string{"--dist", filepath.Join(repository, "internal"), "--target", "linux/amd64"}, want: "enters tracked source"},
		{name: "under tracked internal", args: []string{"--dist", filepath.Join(repository, "internal", "release-output"), "--target", "linux/amd64"}, want: "enters tracked source"},
		{name: "symlink directory", args: []string{"--dist", symlinkDist, "--target", "linux/amd64"}, want: "real directory"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("bash", append([]string{script}, test.args...)...)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("release builder error=%v output=%q want=%q", err, output, test.want)
			}
		})
	}
}

func TestReleaseBuilderInvalidDistDoesNotCreateDirectories(t *testing.T) {
	repository := releaseRepositoryRoot(t)
	script := filepath.Join(repository, "scripts", "build-release.sh")
	missingParent := filepath.Join(repository, ".agent-cache", "release-builder-missing-parent")
	if err := os.RemoveAll(missingParent); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", script, "--dist", filepath.Join(missingParent, "child"), "--target", "linux/amd64")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "parent is unavailable") {
		t.Fatalf("invalid dist error=%v output=%q", err, output)
	}
	if _, statErr := os.Stat(missingParent); !os.IsNotExist(statErr) {
		t.Fatalf("invalid request created output state: %v", statErr)
	}
}

func TestReleaseBuilderRejectsTrackedGeneratedArtifacts(t *testing.T) {
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	paths := []string{
		".agent-cache/go/7f/cached-object-a",
		"internal/pkg/__pycache__/module.pyc",
		"build/omnidex",
		"dist/archive.tar.gz",
		"bin/omni",
		"core",
		"core.817",
		"go.mod.orig",
		"notes.txt~",
		"object.o",
		"omnidex.exe",
	}
	for _, relative := range paths {
		relative := relative
		t.Run(relative, func(t *testing.T) {
			repository := t.TempDir()
			runReleaseGit(t, repository, "init")
			path := filepath.Join(repository, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			writeReleaseTestFile(t, path, "generated\n")
			runReleaseGit(t, repository, "add", "-f", "--", relative)
			command := exec.Command(
				"bash", "-c", `source "$1"; validate_tracked_release_sources "$2"`,
				"release-tracked-source-test", script, repository,
			)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "tracked generated artifact") ||
				!strings.Contains(string(output), relative) {
				t.Fatalf("tracked artifact error=%v output=%q", err, output)
			}
		})
	}
}

func TestReleaseBuilderAcceptsTrackedSourceFiles(t *testing.T) {
	repository := t.TempDir()
	runReleaseGit(t, repository, "init")
	for relative, content := range map[string]string{
		"go.mod":                    "module example.invalid/release\n",
		"cmd/example/main.go":       "package main\n",
		"scripts/release-helper.sh": "#!/usr/bin/env bash\n",
	} {
		path := filepath.Join(repository, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeReleaseTestFile(t, path, content)
		runReleaseGit(t, repository, "add", "--", relative)
	}
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	command := exec.Command(
		"bash", "-c", `source "$1"; validate_tracked_release_sources "$2"`,
		"release-tracked-source-test", script, repository,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("valid tracked sources error=%v output=%q", err, output)
	}
}

func releaseRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeReleaseTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
