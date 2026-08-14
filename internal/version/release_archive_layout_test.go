package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeReleaseArchiveContainsOneRunnableManagedRuntimeLayout(t *testing.T) {
	repository := releaseRepositoryRoot(t)
	script := filepath.Join(repository, "scripts", "build-release.sh")
	targetSource := filepath.Join(t.TempDir(), "source")
	targetDir := filepath.Join(t.TempDir(), "omnidex-v1.2.3-linux-amd64")
	if err := os.MkdirAll(filepath.Join(targetSource, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"default.env", "install-release.sh",
		"scripts/install-shell-lib.sh", "scripts/managed-release-install-lib.sh",
	} {
		source := filepath.Join(repository, path)
		target := filepath.Join(targetSource, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, raw, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, binary := range []string{"omni", "agent-cli", "agent-core"} {
		path := filepath.Join(targetDir, "bin", binary)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	command := exec.Command(
		"bash", "-c", `source "$1"; copy_managed_runtime_layout "$2" "$3" linux`,
		"release-runtime-layout", script, targetSource, targetDir,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("copy native release layout: %v: %s", err, output)
	}

	for _, path := range []string{
		"bin/omni", "bin/agent-cli", "bin/agent-core", "bin/acli", "default.env",
		"install-release.sh", "scripts/install-shell-lib.sh", "scripts/managed-release-install-lib.sh",
	} {
		if _, err := os.Lstat(filepath.Join(targetDir, path)); err != nil {
			t.Fatalf("native release runtime omits %s: %v", path, err)
		}
	}
	for _, path := range []string{"bin/omni", "bin/agent-cli", "bin/agent-core", "install-release.sh"} {
		info, err := os.Stat(filepath.Join(targetDir, path))
		if err != nil || info.Mode()&0o111 == 0 {
			t.Fatalf("native release runtime entry %s is not executable: %v", path, err)
		}
	}
	link, err := os.Readlink(filepath.Join(targetDir, "bin", "acli"))
	if err != nil || link != "agent-cli" {
		t.Fatalf("native release acli link = %q, error = %v", link, err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, ".env")); !os.IsNotExist(err) {
		t.Fatalf("release archive must not publish an unreviewed active .env: %v", err)
	}
	for _, path := range []string{
		".env.example", "agent_aliases.sh", "docker-compose.yml", "install.sh", "update.sh",
		"scripts/managed-checkout-lib.sh", "scripts/update-runtime-lib.sh",
	} {
		if _, err := os.Stat(filepath.Join(targetDir, path)); !os.IsNotExist(err) {
			t.Fatalf("native binary archive retained non-authoritative source-checkout path %s: %v", path, err)
		}
	}
	archiveOutput := t.TempDir()
	command = exec.Command(
		"bash", "-c", `source "$1"; RELEASE_OUTPUT_STAGE="$3"; archive_target "$2" native-runtime linux`,
		"release-native-archive", script, targetDir, archiveOutput,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("archive native release layout: %v: %s", err, output)
	}
	command = exec.Command("tar", "-tzf", filepath.Join(archiveOutput, "native-runtime.tar.gz"))
	raw, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("list native archive: %v: %s", err, raw)
	}
	listing := string(raw)
	for _, path := range []string{"./bin/omni", "./bin/agent-cli", "./bin/agent-core", "./default.env", "./install-release.sh"} {
		if !strings.Contains(listing, path+"\n") {
			t.Fatalf("native archive omits %s:\n%s", path, listing)
		}
	}
	if strings.Contains(listing, "./.env\n") {
		t.Fatalf("native archive contains active .env:\n%s", listing)
	}
}

func TestWindowsReleaseArchiveContainsManagedRuntimeAndNativeCLIEntrypoint(t *testing.T) {
	repository := releaseRepositoryRoot(t)
	script := filepath.Join(repository, "scripts", "build-release.sh")
	targetSource := filepath.Join(t.TempDir(), "source")
	targetDir := filepath.Join(t.TempDir(), "omnidex-v1.2.3-windows-amd64")
	if err := os.MkdirAll(filepath.Join(targetSource, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"default.env", "install-release.sh",
		"scripts/install-shell-lib.sh", "scripts/managed-release-install-lib.sh",
	} {
		raw, err := os.ReadFile(filepath.Join(repository, path))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(targetSource, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, raw, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(targetDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "bin", "agent-cli.exe"), []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		"bash", "-c", `source "$1"; copy_managed_runtime_layout "$2" "$3" windows`,
		"release-windows-runtime-layout", script, targetSource, targetDir,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Windows managed runtime error = %v, output = %q", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(targetDir, "bin", "acli.exe"))
	if err != nil || string(raw) != "binary" {
		t.Fatalf("Windows acli entrypoint error = %v, content = %q", err, raw)
	}
}
