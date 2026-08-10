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
		{name: "tracked migrations", args: []string{"--dist", filepath.Join(repository, "migrations"), "--target", "linux/amd64"}, want: "enters tracked source"},
		{name: "tracked internal", args: []string{"--dist", filepath.Join(repository, "internal"), "--target", "linux/amd64"}, want: "enters tracked source"},
		{name: "under tracked migrations", args: []string{"--dist", filepath.Join(repository, "migrations", "output"), "--target", "linux/amd64"}, want: "enters tracked source"},
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

func TestReleaseBuilderRejectsUnregisteredMigrationEntries(t *testing.T) {
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	tests := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "extra file", setup: func(t *testing.T, source string) {
			writeReleaseTestFile(t, filepath.Join(source, "notes.txt"), "not registered")
		}},
		{name: "subdirectory", setup: func(t *testing.T, source string) {
			if err := os.Mkdir(filepath.Join(source, "nested"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink", setup: func(t *testing.T, source string) {
			if err := os.Symlink("001_valid.sql", filepath.Join(source, "002_link.sql")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, target := t.TempDir(), t.TempDir()
			writeReleaseTestFile(t, filepath.Join(source, "001_valid.sql"), "SELECT 1;\n")
			test.setup(t, source)
			output, err := runMigrationManifest(script, source, target)
			if err == nil || !strings.Contains(output, "migration") {
				t.Fatalf("migration validation error=%v output=%q", err, output)
			}
			if _, statErr := os.Stat(filepath.Join(target, "SHA256SUMS")); !os.IsNotExist(statErr) {
				t.Fatalf("invalid source emitted a manifest: %v", statErr)
			}
		})
	}
}

func TestReleaseBuilderWritesManifestForExactMigrationSet(t *testing.T) {
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	source, target := t.TempDir(), t.TempDir()
	writeReleaseTestFile(t, filepath.Join(source, "001_valid.sql"), "SELECT 1;\n")
	writeReleaseTestFile(t, filepath.Join(source, "002_next.sql"), "SELECT 2;\n")
	if output, err := runMigrationManifest(script, source, target); err != nil {
		t.Fatalf("write exact migration manifest: %v: %s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(target, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n"); len(lines) != 2 ||
		!strings.HasSuffix(lines[0], "  001_valid.sql") || !strings.HasSuffix(lines[1], "  002_next.sql") {
		t.Fatalf("manifest=%q", raw)
	}
}

func runMigrationManifest(script, source, target string) (string, error) {
	command := exec.Command("bash", "-c", `source "$1"; write_migration_manifest "$2" "$3"`,
		"release-builder-test", script, source, target)
	output, err := command.CombinedOutput()
	return string(output), err
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
