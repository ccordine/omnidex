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
	expectedCommit := strings.Repeat("d", 40)
	expectedUser := "1000:1001"

	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
command_exists() { command -v "$1" >/dev/null 2>&1; }
die() { printf '%s\n' "$*" >&2; exit 1; }
log() { :; }
DOCKER_CONTEXT_NAME="production.context"
COMPOSE_PROJECT="system-project"
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT}"
HOST_UID=1000
HOST_GID=1001
export HOST_UID HOST_GID
NO_BUILD=0
NO_CACHE=0
NO_RESTART=0
HOST_ONLY=0
commit="$2"
runtime_user="$3"
compose_cmd="$(resolve_compose_cmd)"
compose_build "$1" "$compose_cmd" "$1/docker-compose.yml" core "$commit"
expected="$(compose_image_id "$1" "$compose_cmd" "$1/docker-compose.yml" core "$commit")"
compose_require_image_commit "$expected" "$commit" "$runtime_user"
compose_restart "$1" "$compose_cmd" "$1/docker-compose.yml" core "$commit"
compose_require_running_image "$1" "$compose_cmd" "$1/docker-compose.yml" core "$expected" "$commit" "$runtime_user"
`
	command := exec.Command("bash", "-c", script, "compose-runtime-authority", root, expectedCommit, expectedUser)
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{
		"PATH":                        fakeBin + ":" + os.Getenv("PATH"),
		"OMNI_TEST_DOCKER_LOG":        logPath,
		"OMNI_TEST_EXPECTED_IMAGE":    expectedImage,
		"OMNI_TEST_RUNNING_IMAGE":     expectedImage,
		"OMNI_TEST_RUNNING_CONTAINER": containerID,
		"OMNI_TEST_IMAGE_COMMIT":      expectedCommit,
		"OMNI_TEST_IMAGE_USER":        expectedUser,
		"OMNI_TEST_RUNNING_COMMIT":    expectedCommit,
		"OMNI_TEST_RUNNING_USER":      expectedUser,
		"OMNI_TEST_HEALTH_COMMIT":     expectedCommit,
		"HOST_UID":                    "9000",
		"HOST_GID":                    "9001",
		"COMPOSE_PROJECT_NAME":        "ambient-project",
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
		if !strings.HasPrefix(line, "production.context|system-project|1000|1001|") {
			t.Fatalf("Docker context and host identity were not preserved: %q", line)
		}
	}
	ordered := []string{
		"docker compose version",
		"docker compose -p system-project -f " + root + "/docker-compose.yml build --pull core",
		"docker compose -p system-project -f " + root + "/docker-compose.yml images -q core",
		"docker image inspect --format {{ index .Config.Labels \"org.opencontainers.image.revision\" }} " + expectedImage,
		"docker image inspect --format {{.Config.User}} " + expectedImage,
		"docker compose -p system-project -f " + root + "/docker-compose.yml up -d --remove-orphans --wait --wait-timeout 180 core",
		"docker compose -p system-project -f " + root + "/docker-compose.yml ps -q core",
		"docker inspect --type container --format {{.Image}} " + containerID,
		"docker inspect --type container --format {{ index .Config.Labels \"org.opencontainers.image.revision\" }} " + containerID,
		"docker inspect --type container --format {{.Config.User}} " + containerID,
		"docker exec " + containerID + " /usr/local/bin/agent-core release:verify-running-health " + expectedCommit,
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
	expectedCommit := strings.Repeat("d", 40)
	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
log() { :; }
DOCKER_CONTEXT_NAME="production.context"
COMPOSE_PROJECT="system-project"
HOST_UID=1000
HOST_GID=1001
export HOST_UID HOST_GID
compose_require_running_image "$1" "docker compose" "$1/docker-compose.yml" core "$2" "$3" "$4"
`
	command := exec.Command("bash", "-c", script, "compose-image-mismatch", root, expectedImage, expectedCommit, "1000:1001")
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{
		"PATH":                        fakeBin + ":" + os.Getenv("PATH"),
		"OMNI_TEST_DOCKER_LOG":        logPath,
		"OMNI_TEST_EXPECTED_IMAGE":    expectedImage,
		"OMNI_TEST_RUNNING_IMAGE":     runningImage,
		"OMNI_TEST_RUNNING_CONTAINER": containerID,
		"OMNI_TEST_RUNNING_COMMIT":    expectedCommit,
		"OMNI_TEST_IMAGE_USER":        "1000:1001",
		"OMNI_TEST_RUNNING_USER":      "1000:1001",
		"OMNI_TEST_HEALTH_COMMIT":     expectedCommit,
	})
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "is running image "+runningImage+", expected "+expectedImage) {
		t.Fatalf("image mismatch error = %v, output = %q", err, output)
	}
}

func TestManagedComposeRuntimeRejectsReleaseIdentityMismatches(t *testing.T) {
	root := repoRootFromOmniTest(t)
	expectedImage := "sha256:" + strings.Repeat("a", 64)
	containerID := strings.Repeat("b", 64)
	expectedCommit := strings.Repeat("d", 40)
	differentCommit := strings.Repeat("e", 40)

	for _, test := range []struct {
		name      string
		operation string
		overrides map[string]string
		want      string
	}{
		{
			name:      "built image label",
			operation: "image",
			overrides: map[string]string{"OMNI_TEST_IMAGE_COMMIT": differentCommit},
			want:      "has release commit " + differentCommit + ", expected " + expectedCommit,
		},
		{
			name:      "built image runtime user",
			operation: "image",
			overrides: map[string]string{"OMNI_TEST_IMAGE_USER": "1002:1003"},
			want:      "has runtime user 1002:1003, expected 1000:1001",
		},
		{
			name:      "running container label",
			operation: "running",
			overrides: map[string]string{"OMNI_TEST_RUNNING_COMMIT": differentCommit},
			want:      "is running release commit " + differentCommit + ", expected " + expectedCommit,
		},
		{
			name:      "running container runtime user",
			operation: "running",
			overrides: map[string]string{"OMNI_TEST_RUNNING_USER": "1002:1003"},
			want:      "is running as 1002:1003, expected 1000:1001",
		},
		{
			name:      "health release commit",
			operation: "running",
			overrides: map[string]string{"OMNI_TEST_HEALTH_COMMIT": differentCommit},
			want:      "health reports release commit " + differentCommit + ", expected " + expectedCommit,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fakeBin, logPath := writeFakeComposePlugin(t)
			script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
log() { :; }
DOCKER_CONTEXT_NAME="production.context"
COMPOSE_PROJECT="system-project"
HOST_UID=1000
HOST_GID=1001
export HOST_UID HOST_GID
if [[ "$4" == "image" ]]; then
  compose_require_image_commit "$2" "$3" "$5"
else
  compose_require_running_image "$1" "docker compose" "$1/docker-compose.yml" core "$2" "$3" "$5"
fi
`
			values := map[string]string{
				"PATH":                        fakeBin + ":" + os.Getenv("PATH"),
				"OMNI_TEST_DOCKER_LOG":        logPath,
				"OMNI_TEST_EXPECTED_IMAGE":    expectedImage,
				"OMNI_TEST_RUNNING_IMAGE":     expectedImage,
				"OMNI_TEST_RUNNING_CONTAINER": containerID,
				"OMNI_TEST_IMAGE_COMMIT":      expectedCommit,
				"OMNI_TEST_IMAGE_USER":        "1000:1001",
				"OMNI_TEST_RUNNING_COMMIT":    expectedCommit,
				"OMNI_TEST_RUNNING_USER":      "1000:1001",
				"OMNI_TEST_HEALTH_COMMIT":     expectedCommit,
			}
			for key, value := range test.overrides {
				values[key] = value
			}
			command := exec.Command("bash", "-c", script, "compose-release-mismatch", root, expectedImage, expectedCommit, test.operation, "1000:1001")
			command.Env = exactTestEnvironment(os.Environ(), values)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("release mismatch error=%v output=%q want %q", err, output, test.want)
			}
			if test.name == "running container runtime user" {
				raw, readErr := os.ReadFile(logPath)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if strings.Contains(string(raw), "release:verify-running-health") {
					t.Fatal("health verification ran after stale container runtime user")
				}
			}
		})
	}
}

func TestManagedComposeRuntimeRejectsInvalidCommitBeforeDocker(t *testing.T) {
	root := repoRootFromOmniTest(t)
	fakeBin, logPath := writeFakeComposePlugin(t)
	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
log() { :; }
DOCKER_CONTEXT_NAME="production.context"
COMPOSE_PROJECT="system-project"
NO_BUILD=0
NO_CACHE=0
compose_build "$1" "docker compose" "$1/docker-compose.yml" core invalid
`
	command := exec.Command("bash", "-c", script, "compose-invalid-commit", root)
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{
		"PATH":                 fakeBin + ":" + os.Getenv("PATH"),
		"OMNI_TEST_DOCKER_LOG": logPath,
	})
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "OMNIDEX_COMMIT must be exactly 40 or 64") {
		t.Fatalf("invalid commit error=%v output=%q", err, output)
	}
	if _, statErr := os.Stat(logPath); !os.IsNotExist(statErr) {
		t.Fatalf("invalid commit invoked Docker: %v", statErr)
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
	DOCKER_CONTEXT_NAME="rootless"
	COMPOSE_PROJECT="omni-nxt"
	HOST_UID=1000
	HOST_GID=1001
	export HOST_UID HOST_GID
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

func TestManagedComposeRuntimeRejectsAmbientDockerAuthority(t *testing.T) {
	root := repoRootFromOmniTest(t)
	fakeBin, logPath := writeFakeComposePlugin(t)
	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
command_exists() { command -v "$1" >/dev/null 2>&1; }
DOCKER_CONTEXT_NAME=""
COMPOSE_PROJECT="omni-nxt"
resolve_compose_cmd
`
	command := exec.Command("bash", "-c", script, "compose-explicit-context", root)
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{
		"PATH":                 fakeBin + ":" + os.Getenv("PATH"),
		"OMNI_TEST_DOCKER_LOG": logPath,
	})
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "DOCKER_CONTEXT must be explicit and non-empty") {
		t.Fatalf("ambient Docker authority error = %v, output = %q", err, output)
	}
	if _, statErr := os.Stat(logPath); !os.IsNotExist(statErr) {
		t.Fatalf("ambient Docker command was invoked: %v", statErr)
	}
}

func TestManagedComposeDeploymentIdentityFailsWithoutOneExactProjectAuthority(t *testing.T) {
	root := repoRootFromOmniTest(t)
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "missing context key", raw: "COMPOSE_PROJECT_NAME=omnidex\nHOST_UID=1000\nHOST_GID=1001\n", want: "DOCKER_CONTEXT exactly once"},
		{name: "duplicate context key", raw: "DOCKER_CONTEXT=one\nDOCKER_CONTEXT=two\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_UID=1000\nHOST_GID=1001\n", want: "DOCKER_CONTEXT exactly once"},
		{name: "blank context", raw: "DOCKER_CONTEXT=\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_UID=1000\nHOST_GID=1001\n", want: "DOCKER_CONTEXT must be explicit and non-empty"},
		{name: "missing project key", raw: "DOCKER_CONTEXT=rootless\nHOST_UID=1000\nHOST_GID=1001\n", want: "COMPOSE_PROJECT_NAME exactly once"},
		{name: "blank project", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=\nHOST_UID=1000\nHOST_GID=1001\n", want: "COMPOSE_PROJECT_NAME must be explicit and non-empty"},
		{name: "invalid project", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=bad/project\nHOST_UID=1000\nHOST_GID=1001\n", want: "unsupported characters"},
		{name: "missing host uid", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_GID=1001\n", want: "HOST_UID exactly once"},
		{name: "zero host uid", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_UID=0\nHOST_GID=1001\n", want: "HOST_UID must be one exact positive"},
		{name: "noncanonical host uid", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_UID=01000\nHOST_GID=1001\n", want: "HOST_UID must be one exact positive"},
		{name: "padded host uid", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_UID=1000 \nHOST_GID=1001\n", want: "HOST_UID must be one exact positive"},
		{name: "oversized host uid", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_UID=4294967295\nHOST_GID=1001\n", want: "HOST_UID must be one exact positive"},
		{name: "missing host gid", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_UID=1000\n", want: "HOST_GID exactly once"},
		{name: "invalid host gid", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=omnidex\nHOST_UID=1000\nHOST_GID=group\n", want: "HOST_GID must be one exact positive"},
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
managed_checkout_require_env_key "$2" HOST_UID
managed_checkout_require_env_key "$2" HOST_GID
DOCKER_CONTEXT_NAME="$(managed_checkout_env_value "$2" DOCKER_CONTEXT)"
COMPOSE_PROJECT="$(managed_checkout_env_value "$2" COMPOSE_PROJECT_NAME)"
HOST_UID_VALUE="$(managed_checkout_env_value "$2" HOST_UID)"
HOST_GID_VALUE="$(managed_checkout_env_value "$2" HOST_GID)"
validate_compose_identity DOCKER_CONTEXT "$DOCKER_CONTEXT_NAME"
validate_compose_identity COMPOSE_PROJECT_NAME "$COMPOSE_PROJECT"
[[ -n "$DOCKER_CONTEXT_NAME" ]] || die "DOCKER_CONTEXT must be explicit and non-empty"
[[ -n "$COMPOSE_PROJECT" ]] || die "COMPOSE_PROJECT_NAME must be explicit and non-empty"
runtime_user_identity "$HOST_UID_VALUE" "$HOST_GID_VALUE" >/dev/null
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
printf '%s|%s|%s|%s|docker %s\n' "${DOCKER_CONTEXT:-}" "${COMPOSE_PROJECT_NAME:-}" "${HOST_UID:-}" "${HOST_GID:-}" "$*" >> "${OMNI_TEST_DOCKER_LOG}"
case "$*" in
  "compose version") exit 0 ;;
  *" images -q core") printf '%s\n' "${OMNI_TEST_EXPECTED_IMAGE}" ;;
  *" ps -q core") printf '%s\n' "${OMNI_TEST_RUNNING_CONTAINER}" ;;
	  "image inspect --format {{ index .Config.Labels \"org.opencontainers.image.revision\" }} "*) printf '%s\n' "${OMNI_TEST_IMAGE_COMMIT}" ;;
	  "image inspect --format {{.Config.User}} "*) printf '%s\n' "${OMNI_TEST_IMAGE_USER}" ;;
	  "inspect --type container --format {{.Image}} "*) printf '%s\n' "${OMNI_TEST_RUNNING_IMAGE}" ;;
	  "inspect --type container --format {{ index .Config.Labels \"org.opencontainers.image.revision\" }} "*) printf '%s\n' "${OMNI_TEST_RUNNING_COMMIT}" ;;
	  "inspect --type container --format {{.Config.User}} "*) printf '%s\n' "${OMNI_TEST_RUNNING_USER}" ;;
  "exec "*" /usr/local/bin/agent-core release:verify-running-health "*) printf '%s\n' "${OMNI_TEST_HEALTH_COMMIT}" ;;
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
