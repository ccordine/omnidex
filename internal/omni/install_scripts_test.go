package omni

import (
	"os"
	"os/exec"
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

func TestInstallAndUpdateNeverPromoteEnvironmentTemplate(t *testing.T) {
	root := repoRootFromOmniTest(t)
	combined := readRepoScript(t, root, "install.sh") +
		readRepoScript(t, root, "update.sh") +
		readRepoScript(t, root, "scripts/managed-checkout-lib.sh")
	for _, required := range []string{
		"--env-file", "managed_checkout_stage_env", "default.env is a template only",
		"cannot replace an existing managed .env", "regular non-symlink file",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("managed checkout environment authority omits %q", required)
		}
	}
	for _, forbidden := range []string{
		"managed_checkout_preserve_env",
		`cp -p "${stage}/default.env" "${stage}/.env"`,
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("managed checkout retains template-promotion path %q", forbidden)
		}
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
		"managed_checkout_require_env_key",
		"managed_checkout_env_value",
		"DOCKER_CONTEXT",
		"COMPOSE_PROJECT_NAME",
		"validate_compose_identity",
		"output+=(-p",
		"COMPOSE_PROJECT_NAME must be explicit and non-empty",
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

func TestManagedUpdateWaitsForOneHealthyCoreDeployment(t *testing.T) {
	root := repoRootFromOmniTest(t)
	runtime := readRepoScript(t, root, "scripts/update-runtime-lib.sh")
	compose := readRepoScript(t, root, "docker-compose.yml")
	update := readRepoScript(t, root, "update.sh")

	for _, required := range []string{
		"docker compose", "up -d --remove-orphans", "--wait", "--wait-timeout",
		"compose_image_id", "compose_require_running_image", `.Image`,
	} {
		if !strings.Contains(runtime, required) {
			t.Fatalf("managed update omits health-gated compose fragment %q", required)
		}
	}
	if strings.Contains(runtime, "docker-compose") {
		t.Fatal("managed update retains a second docker-compose implementation")
	}
	for _, required := range []string{"healthcheck:", "/readyz", "start_period:", "retries:"} {
		if !strings.Contains(compose, required) {
			t.Fatalf("core deployment omits health authority %q", required)
		}
	}
	restart := strings.Index(update, "compose_restart")
	success := strings.LastIndex(update, `log "update complete"`)
	if restart < 0 || success < 0 || restart >= success {
		t.Fatal("managed update can report success before the health-gated restart")
	}
}

func TestManagedUpdateCannotReportSuccessWhenComposeHealthWaitFails(t *testing.T) {
	root := repoRootFromOmniTest(t)
	bin := t.TempDir()
	fakeDocker := filepath.Join(bin, "docker")
	if err := os.WriteFile(fakeDocker, []byte("#!/usr/bin/env sh\nexit 17\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
log() { printf '%s\n' "$*"; }
die() { printf '%s\n' "$*" >&2; exit 1; }
NO_RESTART=0
DOCKER_CONTEXT_NAME=""
COMPOSE_PROJECT=""
compose_restart "$1" "docker compose" "$1/docker-compose.yml" core
printf '%s\n' 'update complete'
`
	command := exec.Command("bash", "-c", script, "update-health-test", root)
	command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("failed compose readiness wait returned success: %s", output)
	}
	if strings.Contains(string(output), "update complete") {
		t.Fatalf("failed compose readiness wait reported success: %s", output)
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
