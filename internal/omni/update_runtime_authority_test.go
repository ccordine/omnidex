package omni

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedComposeRuntimePreservesContextProjectHealthAndImageAuthority(t *testing.T) {
	root := repoRootFromOmniTest(t)
	fakeBin, logPath := writeFakeComposePlugin(t)
	expectedImage := "sha256:" + strings.Repeat("a", 64)
	containerID := strings.Repeat("b", 64)

	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
command_exists() { command -v "$1" >/dev/null 2>&1; }
die() { printf '%s\n' "$*" >&2; exit 1; }
log() { :; }
DOCKER_CONTEXT_NAME="production.context"
COMPOSE_PROJECT="system-project"
NO_BUILD=0
NO_CACHE=0
NO_RESTART=0
HOST_ONLY=0
compose_cmd="$(resolve_compose_cmd)"
compose_build "$1" "$compose_cmd" "$1/docker-compose.yml" core
expected="$(compose_image_id "$1" "$compose_cmd" "$1/docker-compose.yml" core)"
compose_restart "$1" "$compose_cmd" "$1/docker-compose.yml" core
compose_require_running_image "$1" "$compose_cmd" "$1/docker-compose.yml" core "$expected"
`
	command := exec.Command("bash", "-c", script, "compose-runtime-authority", root)
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{
		"PATH":                        fakeBin + ":" + os.Getenv("PATH"),
		"OMNI_TEST_DOCKER_LOG":        logPath,
		"OMNI_TEST_EXPECTED_IMAGE":    expectedImage,
		"OMNI_TEST_RUNNING_IMAGE":     expectedImage,
		"OMNI_TEST_RUNNING_CONTAINER": containerID,
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("managed compose runtime: %v: %s", err, output)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
		if !strings.HasPrefix(line, "production.context|") {
			t.Fatalf("Docker context was not preserved: %q", line)
		}
	}
	ordered := []string{
		"docker compose version",
		"docker compose -p system-project -f " + root + "/docker-compose.yml build --pull core",
		"docker compose -p system-project -f " + root + "/docker-compose.yml images -q core",
		"docker compose -p system-project -f " + root + "/docker-compose.yml up -d --remove-orphans --wait --wait-timeout 180 core",
		"docker compose -p system-project -f " + root + "/docker-compose.yml ps -q core",
		"docker inspect --format {{.Image}} " + containerID,
	}
	position := -1
	for _, fragment := range ordered {
		next := strings.Index(log[position+1:], fragment)
		if next < 0 {
			t.Fatalf("managed Compose log omits ordered operation %q:\n%s", fragment, log)
		}
		position += next + 1
	}
}

func TestManagedComposeRuntimeRejectsRunningImageMismatch(t *testing.T) {
	root := repoRootFromOmniTest(t)
	fakeBin, logPath := writeFakeComposePlugin(t)
	expectedImage := "sha256:" + strings.Repeat("a", 64)
	runningImage := "sha256:" + strings.Repeat("c", 64)
	containerID := strings.Repeat("b", 64)
	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
log() { :; }
DOCKER_CONTEXT_NAME="production.context"
COMPOSE_PROJECT="system-project"
compose_require_running_image "$1" "docker compose" "$1/docker-compose.yml" core "$2"
`
	command := exec.Command("bash", "-c", script, "compose-image-mismatch", root, expectedImage)
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{
		"PATH":                        fakeBin + ":" + os.Getenv("PATH"),
		"OMNI_TEST_DOCKER_LOG":        logPath,
		"OMNI_TEST_EXPECTED_IMAGE":    expectedImage,
		"OMNI_TEST_RUNNING_IMAGE":     runningImage,
		"OMNI_TEST_RUNNING_CONTAINER": containerID,
	})
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "is running image "+runningImage+", expected "+expectedImage) {
		t.Fatalf("image mismatch error = %v, output = %q", err, output)
	}
}

func TestManagedComposeRuntimeRejectsLegacyStandaloneBinary(t *testing.T) {
	root := repoRootFromOmniTest(t)
	fakeBin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "legacy-invoked")
	legacy := "#!/usr/bin/env bash\ntouch " + shellQuote(marker) + "\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "docker-compose"), []byte(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
command_exists() { command -v "$1" >/dev/null 2>&1; }
die() { printf '%s\n' "$*" >&2; exit 1; }
DOCKER_CONTEXT_NAME=""
resolve_compose_cmd
`
	command := exec.Command("bash", "-c", script, "compose-plugin-only", root)
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{"PATH": fakeBin})
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Docker Compose plugin is required") {
		t.Fatalf("standalone Compose rejection = %v, output = %q", err, output)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("legacy docker-compose was invoked: %v", statErr)
	}
}

func TestManagedComposeDeploymentIdentityFailsWithoutOneExactProjectAuthority(t *testing.T) {
	root := repoRootFromOmniTest(t)
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "missing context key", raw: "COMPOSE_PROJECT_NAME=omnidex\n", want: "DOCKER_CONTEXT exactly once"},
		{name: "duplicate context key", raw: "DOCKER_CONTEXT=one\nDOCKER_CONTEXT=two\nCOMPOSE_PROJECT_NAME=omnidex\n", want: "DOCKER_CONTEXT exactly once"},
		{name: "missing project key", raw: "DOCKER_CONTEXT=\n", want: "COMPOSE_PROJECT_NAME exactly once"},
		{name: "blank project", raw: "DOCKER_CONTEXT=\nCOMPOSE_PROJECT_NAME=\n", want: "explicit and non-empty"},
		{name: "invalid project", raw: "DOCKER_CONTEXT=\nCOMPOSE_PROJECT_NAME=bad/project\n", want: "unsupported characters"},
	} {
		t.Run(test.name, func(t *testing.T) {
			environment := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(environment, []byte(test.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			script := `
set -euo pipefail
source "$1/scripts/managed-checkout-lib.sh"
source "$1/scripts/update-runtime-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
managed_checkout_require_env_key "$2" DOCKER_CONTEXT
managed_checkout_require_env_key "$2" COMPOSE_PROJECT_NAME
DOCKER_CONTEXT_NAME="$(managed_checkout_env_value "$2" DOCKER_CONTEXT)"
COMPOSE_PROJECT="$(managed_checkout_env_value "$2" COMPOSE_PROJECT_NAME)"
validate_compose_identity DOCKER_CONTEXT "$DOCKER_CONTEXT_NAME"
validate_compose_identity COMPOSE_PROJECT_NAME "$COMPOSE_PROJECT"
[[ -n "$COMPOSE_PROJECT" ]] || die "COMPOSE_PROJECT_NAME must be explicit and non-empty"
`
			command := exec.Command("bash", "-c", script, "compose-deployment-identity", root, environment)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("deployment identity error = %v, output = %q", err, output)
			}
		})
	}
}

func writeFakeComposePlugin(t *testing.T) (string, string) {
	t.Helper()
	fakeBin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	contents := `#!/usr/bin/env bash
set -euo pipefail
printf '%s|docker %s\n' "${DOCKER_CONTEXT:-}" "$*" >> "${OMNI_TEST_DOCKER_LOG}"
case "$*" in
  "compose version") exit 0 ;;
  *" images -q core") printf '%s\n' "${OMNI_TEST_EXPECTED_IMAGE}" ;;
  *" ps -q core") printf '%s\n' "${OMNI_TEST_RUNNING_CONTAINER}" ;;
  "inspect --format {{.Image}} "*) printf '%s\n' "${OMNI_TEST_RUNNING_IMAGE}" ;;
  *" build --pull core"|*" up -d --remove-orphans --wait --wait-timeout 180 core") exit 0 ;;
  *) printf 'unexpected docker invocation: %s\n' "$*" >&2; exit 72 ;;
esac
`
	if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	return fakeBin, logPath
}

func exactTestEnvironment(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if _, replace := overrides[key]; found && replace {
			continue
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}
