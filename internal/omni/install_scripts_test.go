package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAndUpdateScriptsBuildOmniBinary(t *testing.T) {
	root := repoRootFromOmniTest(t)

	for _, scriptName := range []string{"install.sh", "update.sh"} {
		body := readRepoScript(t, root, scriptName)
		if !strings.Contains(body, "go build -o bin/omni ./cmd/omni") {
			t.Fatalf("%s must build bin/omni from ./cmd/omni", scriptName)
		}
	}
}

func TestInstallScriptAddsBinDirectoryToPath(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "install.sh")

	for _, want := range []string{
		"export OMNIDEX_DIR=\"${PREFIX}\"",
		"export PATH=\"\\$OMNIDEX_DIR/bin:\\$PATH\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install.sh missing PATH integration fragment %q", want)
		}
	}
}

func TestInstallScriptCopiesPublicRuntimeResources(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "install.sh")

	for _, want := range []string{
		"recipes",
		"benchmarks",
		"docs",
		"SECURITY.md",
		"LICENSE",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install.sh runtime payload missing %q", want)
		}
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Fatalf("runtime payload item %s must exist in repo: %v", want, err)
		}
	}
}

func TestCrossPlatformBootstrapScriptsArePackaged(t *testing.T) {
	root := repoRootFromOmniTest(t)
	installBody := readRepoScript(t, root, "install.sh")
	updateBody := readRepoScript(t, root, "update.sh")

	for _, scriptName := range []string{
		"scripts/build-release.sh",
		"scripts/setup-host-deps.ps1",
	} {
		if _, err := os.Stat(filepath.Join(root, scriptName)); err != nil {
			t.Fatalf("cross-platform helper %s must exist in repo: %v", scriptName, err)
		}
		if !strings.Contains(installBody, scriptName) {
			t.Fatalf("install.sh must refresh permissions for %s", scriptName)
		}
		if !strings.Contains(updateBody, scriptName) {
			t.Fatalf("update.sh must refresh permissions for %s", scriptName)
		}
	}
}

func TestUpdateScriptSupportsHostOnlyInstalledUpdate(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "update.sh")

	for _, want := range []string{
		"--host-only",
		"needs_compose_work",
		"refresh_installed_payload_permissions",
		"go build -o bin/omni ./cmd/omni",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("update.sh missing installed-update fragment %q", want)
		}
	}
}

func repoRootFromOmniTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repo root from %s: %v", wd, err)
	}
	return root
}

func readRepoScript(t *testing.T, root, scriptName string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, scriptName))
	if err != nil {
		t.Fatalf("read %s: %v", scriptName, err)
	}
	return string(data)
}
