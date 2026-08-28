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
		if !strings.Contains(body, `-o "${build_dir}/omni" ./cmd/omni`) {
			t.Fatalf("%s must build bin/omni from ./cmd/omni", scriptName)
		}
		for _, fragment := range []string{"go build -trimpath", "internal/version.Commit=${OMNIDEX_COMMIT}", "managed_checkout_verify_binary_commit"} {
			if !strings.Contains(body, fragment) {
				t.Fatalf("%s omits release-bound native build fragment %q", scriptName, fragment)
			}
		}
	}
}

func TestManagedCheckoutBuildCommitOverwritesAmbientWithCleanHead(t *testing.T) {
	root := repoRootFromOmniTest(t)
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "tracked.txt"), "clean\n", 0o600)
	runFixtureGit(t, repository, "init", "-b", "main")
	runFixtureGit(t, repository, "config", "user.email", "identity-test@example.invalid")
	runFixtureGit(t, repository, "config", "user.name", "Identity Test")
	runFixtureGit(t, repository, "add", ".")
	runFixtureGit(t, repository, "commit", "-m", "fixture")
	expected := runFixtureGit(t, repository, "rev-parse", "HEAD")

	script := `
set -euo pipefail
source "$1/scripts/managed-checkout-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
managed_checkout_export_build_commit "$2"
printf '%s\n' "$OMNIDEX_COMMIT"
`
	command := exec.Command("bash", "-c", script, "managed-build-commit", root, repository)
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{"OMNIDEX_COMMIT": strings.Repeat("f", 40)})
	output, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != expected {
		t.Fatalf("managed commit output=%q error=%v want %q", output, err, expected)
	}

	writeFixtureFile(t, filepath.Join(repository, "tracked.txt"), "dirty\n", 0o600)
	command = exec.Command("bash", "-c", script, "managed-build-commit-dirty", root, repository)
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{"OMNIDEX_COMMIT": strings.Repeat("f", 40)})
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "source checkout is dirty") {
		t.Fatalf("dirty checkout commit error=%v output=%q", err, output)
	}
}

func TestManagedCheckoutCleanCheckIncludesSubmodules(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "scripts/managed-checkout-lib.sh")
	for _, fragment := range []string{
		"status --porcelain=v1",
		"--untracked-files=normal",
		"--ignore-submodules=none",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("managed checkout clean-state check omits %q", fragment)
		}
	}
	if strings.Contains(body, "--ignore-submodules --") {
		t.Fatal("managed checkout still ignores submodule modifications")
	}
}

func TestNativeBuildEntrypointsBindAndVerifyOneCleanCommit(t *testing.T) {
	root := repoRootFromOmniTest(t)
	for _, scriptName := range []string{"install.sh", "update.sh", "scripts/build-core.sh"} {
		body := readRepoScript(t, root, scriptName)
		for _, fragment := range []string{
			"managed_checkout_export_build_commit",
			"go build -trimpath",
			"managed_checkout_verify_binary_commit",
		} {
			if !strings.Contains(body, fragment) {
				t.Fatalf("%s omits native release identity fragment %q", scriptName, fragment)
			}
		}
		if count := strings.Count(body, "internal/version.Commit=${OMNIDEX_COMMIT}"); count != 1 {
			t.Fatalf("%s has %d release commit linker assignments, want one authoritative assignment", scriptName, count)
		}
	}
}

func TestShellAliasesUseOnlyManagedReleaseBinaries(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "agent_aliases.sh")
	for _, required := range []string{
		`${OMNIDEX_DIR}/bin/agent-cli`,
		`${OMNIDEX_DIR}/bin/omni`,
		"managed CLI binary is missing",
		"managed omni binary is missing",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("agent aliases omit managed-binary authority %q", required)
		}
	}
	for _, forbidden := range []string{"go run", "command agent-cli", "OMNIDEX_USE_SYSTEM_OMNI", "type -P omni"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("agent aliases retain alternate runtime path %q", forbidden)
		}
	}
}

func TestMakeBuildAndRunUseReleaseBoundBinaries(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "Makefile")
	for _, fragment := range []string{
		"./scripts/build-core.sh --package ./cmd/cli --output bin/agent-cli",
		"./scripts/build-core.sh --package ./cmd/omni --output bin/omni",
		"run: core",
		"./bin/agent-core",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("Makefile omits release-bound entrypoint %q", fragment)
		}
	}
	for _, forbidden := range []string{"go build -o bin/agent-cli", "go build -o bin/omni", "go run ./cmd/core"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Makefile retains unbound build/run entrypoint %q", forbidden)
		}
	}
}

func TestNativeBinaryBuilderBuildsUIOnlyForCore(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "scripts/build-core.sh")
	guard := `if [[ "${BUILD_PKG}" == "./cmd/core" ]]; then`
	guardIndex := strings.Index(body, guard)
	uiIndex := strings.Index(body, `"${SCRIPT_DIR}/build-ui.sh"`)
	if guardIndex < 0 || uiIndex < guardIndex {
		t.Fatal("native binary builder does not gate UI construction to the core package")
	}
	if count := strings.Count(body, `"${SCRIPT_DIR}/build-ui.sh"`); count != 1 {
		t.Fatalf("native binary builder has %d UI build invocations, want one core-only invocation", count)
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
		`-o "${build_dir}/omni" ./cmd/omni`,
		"managed_checkout_export_build_commit",
		"managed_checkout_verify_binary_commit",
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
		if !strings.Contains(body, "DOCKER_CONTEXT=default\n") ||
			!strings.Contains(body, "COMPOSE_PROJECT_NAME=omnidex") ||
			!strings.Contains(body, "DOCKER_GID=\n") {
			t.Fatalf("%s omits explicit Docker deployment identity", profile)
		}
	}
}

func TestUpdateAndQuickstartExposeOnlyReleaseBoundCoreDeployment(t *testing.T) {
	root := repoRootFromOmniTest(t)
	update := readRepoScript(t, root, "update.sh")
	if strings.Contains(update, "--service") || strings.Contains(update, `${SERVICE}`) {
		t.Fatal("update.sh still exposes an unverified arbitrary Compose service")
	}
	for _, fragment := range []string{
		`compose_build "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" core`,
		`compose_restart "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" core`,
	} {
		if !strings.Contains(update, fragment) {
			t.Fatalf("update.sh omits core-only release operation %q", fragment)
		}
	}
	defaults := readRepoScript(t, root, "default.env")
	if !strings.Contains(defaults, "#   ./up.sh --build") ||
		!strings.Contains(defaults, "DOCKER_CONTEXT=default\n") ||
		!strings.Contains(defaults, "DOCKER_GID=\n") {
		t.Fatal("default.env quickstart does not select the release-bound default Docker authority")
	}
	if strings.Contains(defaults, "#   docker compose up --build") {
		t.Fatal("default.env still advertises an unbound direct Compose build")
	}
	if example := readRepoScript(t, root, ".env.example"); !strings.Contains(example, "DOCKER_CONTEXT=default\n") ||
		!strings.Contains(example, "DOCKER_GID=\n") {
		t.Fatal(".env.example does not declare the default Docker authority and required socket group")
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
DOCKER_CONTEXT_NAME="default"
COMPOSE_PROJECT="omni-nxt"
HOST_UID=1000
HOST_GID=1001
export HOST_UID HOST_GID
compose_restart "$1" "docker compose" "$1/docker-compose.yml" core "$2"
printf '%s\n' 'update complete'
`
	command := exec.Command("bash", "-c", script, "update-health-test", root, strings.Repeat("a", 40))
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
