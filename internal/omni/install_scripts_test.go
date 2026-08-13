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
		if !strings.Contains(body, `go build -o "${build_dir}/omni" ./cmd/omni`) {
			t.Fatalf("%s must build bin/omni from ./cmd/omni", scriptName)
		}
	}
}

func TestInstallScriptAddsBinDirectoryToPath(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "install.sh") + readRepoScript(t, root, "scripts/install-shell-lib.sh")

	for _, want := range []string{
		"export OMNIDEX_DIR=\"${PREFIX}\"",
		"export PATH=\"\\$OMNIDEX_DIR/bin:\\$PATH\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install.sh missing PATH integration fragment %q", want)
		}
	}
}

func TestInstallScriptStagesCompleteCheckoutInsteadOfPartialPayload(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "install.sh")
	checkout := readRepoScript(t, root, "scripts/managed-checkout-lib.sh")

	for _, want := range []string{"managed_checkout_clone_exact", "git clone", "ls-files --deleted"} {
		combined := body + checkout
		if !strings.Contains(combined, want) {
			t.Fatalf("complete checkout installer missing %q", want)
		}
	}
	if strings.Contains(body, "payload_items") || strings.Contains(body, "copy_runtime_payload") {
		t.Fatal("installer retains partial payload copying")
	}
}

func TestCompleteCheckoutPackagesCrossPlatformBootstrapScripts(t *testing.T) {
	root := repoRootFromOmniTest(t)

	for _, scriptName := range []string{
		"scripts/build-release.sh",
		"scripts/setup-host-deps.ps1",
	} {
		if _, err := os.Stat(filepath.Join(root, scriptName)); err != nil {
			t.Fatalf("cross-platform helper %s must exist in repo: %v", scriptName, err)
		}
	}
}

func TestUpdateScriptSupportsHostOnlyInstalledUpdate(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "update.sh")
	runtimeLibrary := readRepoScript(t, root, "scripts/update-runtime-lib.sh")
	combined := body + runtimeLibrary

	for _, want := range []string{
		"--host-only",
		"--no-host-restart",
		"needs_compose_work",
		"managed_checkout_fast_forward",
		"managed_checkout_publish",
		"restart_host_bridge",
		`go build -o "${build_dir}/omni" ./cmd/omni`,
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("update.sh missing installed-update fragment %q", want)
		}
	}
}

func TestUpdateScriptConsumesExactDockerDeploymentAuthority(t *testing.T) {
	root := repoRootFromOmniTest(t)
	combined := readRepoScript(t, root, "update.sh") +
		readRepoScript(t, root, "scripts/managed-checkout-lib.sh") +
		readRepoScript(t, root, "scripts/update-runtime-lib.sh")
	for _, fragment := range []string{
		"managed_checkout_env_value",
		"DOCKER_CONTEXT",
		"COMPOSE_PROJECT_NAME",
		"validate_compose_identity",
		"output+=(-p",
	} {
		if !strings.Contains(combined, fragment) {
			t.Fatalf("update deployment authority missing %q", fragment)
		}
	}
	for _, profile := range []string{"default.env", ".env.example"} {
		body := readRepoScript(t, root, profile)
		if !strings.Contains(body, "DOCKER_CONTEXT=") || !strings.Contains(body, "COMPOSE_PROJECT_NAME=omnidex") {
			t.Fatalf("%s omits explicit Docker deployment identity", profile)
		}
	}
}

func TestInstallAndUpdateNeverReportSuccessWithMissingOrPartiallyBuiltBinaries(t *testing.T) {
	root := repoRootFromOmniTest(t)
	for _, scriptName := range []string{"install.sh", "update.sh"} {
		body := readRepoScript(t, root, scriptName)
		for _, forbidden := range []string{
			"skipping host binary rebuild",
			"rm -f bin/agent-core bin/agent-cli bin/omni",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s retains non-atomic/stale binary behavior %q", scriptName, forbidden)
			}
		}
		for _, required := range []string{
			".omnidex-build.XXXXXX",
			"go is required to build Omnidex binaries",
			"scripts/build-ui.sh",
			"managed_checkout_validate_env",
			"managed_checkout_publish",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s omits exact binary update guard %q", scriptName, required)
			}
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
