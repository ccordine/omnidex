package omni

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedComposeImageResolutionUsesCurrentConfiguredServiceImage(t *testing.T) {
	root := repoRootFromOmniTest(t)
	freshImage := "sha256:" + strings.Repeat("a", 64)
	dependencyImage := "sha256:" + strings.Repeat("c", 64)
	staleImage := "sha256:" + strings.Repeat("b", 64)
	commit := strings.Repeat("d", 40)

	for _, test := range []struct {
		name        string
		refs        string
		legacyImage string
	}{
		{
			name: "implicit build image without a container",
			refs: "dependency:16\nimplicit-project-core\n" +
				"implicit-project-core\nmissing:latest\n",
		},
		{
			name:        "explicit build image with a stale container",
			refs:        "registry.example/runtime:current\ndependency:16\n",
			legacyImage: staleImage,
		},
		{
			name: "configured aliases deduplicate one image identity",
			refs: "implicit-project-core\n" +
				"registry.example/runtime:current\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fakeBin, logPath := writeComposeImageResolverDocker(t)
			output, err := runComposeImageResolver(t, root, fakeBin, map[string]string{
				"OMNI_TEST_CONFIG_REFS":      test.refs,
				"OMNI_TEST_CONFIG_STATUS":    "0",
				"OMNI_TEST_FRESH_IMAGE":      freshImage,
				"OMNI_TEST_DEPENDENCY_IMAGE": dependencyImage,
				"OMNI_TEST_ONE_IMAGE":        "",
				"OMNI_TEST_TWO_IMAGE":        "",
				"OMNI_TEST_FRESH_PROJECT":    "exact-project",
				"OMNI_TEST_FRESH_SERVICE":    "core",
				"OMNI_TEST_ONE_PROJECT":      "",
				"OMNI_TEST_ONE_SERVICE":      "",
				"OMNI_TEST_TWO_PROJECT":      "",
				"OMNI_TEST_TWO_SERVICE":      "",
				"OMNI_TEST_LEGACY_IMAGE":     test.legacyImage,
			}, commit)
			if err != nil {
				t.Fatalf("configured image resolution: %v: %s", err, output)
			}
			if got := strings.TrimSpace(string(output)); got != freshImage {
				t.Fatalf("resolved image=%q want fresh rootful image %q", got, freshImage)
			}

			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			log := string(raw)
			for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
				if !strings.HasPrefix(line, "default|<unset>|<unset>|<unset>|") {
					t.Fatalf("image resolution retained ambient Docker routing: %q", line)
				}
			}
			if strings.Contains(log, " images -q core") || strings.Contains(log, " ps -q core") {
				t.Fatalf("image resolution consulted container state: %s", raw)
			}
			if !strings.Contains(log, " config --images core\n") {
				t.Fatalf("configured image references were not requested: %s", raw)
			}
		})
	}
}

func TestManagedComposeImageResolutionRejectsMissingAmbiguousOrMalformedAuthority(t *testing.T) {
	root := repoRootFromOmniTest(t)
	validOne := "sha256:" + strings.Repeat("a", 64)
	validTwo := "sha256:" + strings.Repeat("b", 64)
	dependencyImage := "sha256:" + strings.Repeat("c", 64)
	commit := strings.Repeat("d", 40)

	for _, test := range []struct {
		name       string
		refs       string
		status     string
		oneImage   string
		twoImage   string
		oneProject string
		oneService string
		twoProject string
		twoService string
		want       string
	}{
		{name: "config command failure", status: "71", want: "configured image references are unavailable"},
		{name: "empty config projection", want: "returned no configured image references"},
		{
			name: "no matching service labels", refs: "target-one:latest\n",
			oneImage: validOne, oneProject: "another-project", oneService: "core",
			want: "resolved 0 current configured image identities",
		},
		{name: "configured image not present", refs: "target-one:latest\n", want: "resolved 0 current configured image identities"},
		{
			name: "ambiguous matching images", refs: "target-one:latest\ntarget-two:latest\n",
			oneImage: validOne, twoImage: validTwo,
			oneProject: "exact-project", oneService: "core",
			twoProject: "exact-project", twoService: "core",
			want: "resolved 2 current configured image identities",
		},
		{name: "blank image reference", refs: "target-one:latest\n\nmissing:latest\n", want: "invalid configured image reference"},
		{name: "whitespace image reference", refs: "target one:latest\n", want: "invalid configured image reference"},
		{name: "option image reference", refs: "--format\n", want: "invalid configured image reference"},
		{
			name: "malformed image identity", refs: "target-one:latest\n",
			oneImage: "not-an-image-id", oneProject: "exact-project", oneService: "core",
			want: "returned an invalid image identity",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fakeBin, _ := writeComposeImageResolverDocker(t)
			configStatus := test.status
			if configStatus == "" {
				configStatus = "0"
			}
			output, err := runComposeImageResolver(t, root, fakeBin, map[string]string{
				"OMNI_TEST_CONFIG_REFS":      test.refs,
				"OMNI_TEST_CONFIG_STATUS":    configStatus,
				"OMNI_TEST_FRESH_IMAGE":      "",
				"OMNI_TEST_DEPENDENCY_IMAGE": dependencyImage,
				"OMNI_TEST_ONE_IMAGE":        test.oneImage,
				"OMNI_TEST_TWO_IMAGE":        test.twoImage,
				"OMNI_TEST_FRESH_PROJECT":    "",
				"OMNI_TEST_FRESH_SERVICE":    "",
				"OMNI_TEST_ONE_PROJECT":      test.oneProject,
				"OMNI_TEST_ONE_SERVICE":      test.oneService,
				"OMNI_TEST_TWO_PROJECT":      test.twoProject,
				"OMNI_TEST_TWO_SERVICE":      test.twoService,
				"OMNI_TEST_LEGACY_IMAGE":     "",
			}, commit)
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("image authority error=%v output=%q want %q", err, output, test.want)
			}
		})
	}
}

func runComposeImageResolver(t *testing.T, root, fakeBin string, values map[string]string, commit string) ([]byte, error) {
	t.Helper()
	script := `
set -euo pipefail
source "$1/scripts/update-runtime-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
DOCKER_CONTEXT_NAME=default
COMPOSE_PROJECT=exact-project
HOST_UID=1000
HOST_GID=1001
export HOST_UID HOST_GID
compose_image_id "$1" "docker compose" "$1/docker-compose.yml" core "$2"
`
	command := exec.Command("bash", "-c", script, "compose-image-resolution", root, commit)
	values["PATH"] = fakeBin + ":" + os.Getenv("PATH")
	values["DOCKER_CONTEXT"] = "rootless"
	values["DOCKER_HOST"] = "unix:///run/user/1000/docker.sock"
	values["DOCKER_CONFIG"] = "/tmp/alternate-docker-config"
	values["BUILDX_BUILDER"] = "rootless"
	values["BUILDX_CONFIG"] = "/tmp/alternate-buildx-config"
	values["COMPOSE_PROJECT_NAME"] = "ambient-project"
	command.Env = exactTestEnvironment(os.Environ(), values)
	return command.CombinedOutput()
}

func writeComposeImageResolverDocker(t *testing.T) (string, string) {
	t.Helper()
	fakeBin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	fakeDocker := `#!/usr/bin/env bash
set -euo pipefail
printf '%s|%s|%s|%s|%s\n' \
  "${DOCKER_CONTEXT:-<unset>}" "${DOCKER_HOST:-<unset>}" \
  "${BUILDX_BUILDER:-<unset>}" "${BUILDX_CONFIG:-<unset>}" "$*" \
  >>"${OMNI_TEST_DOCKER_LOG}"
case "$*" in
  'compose -p exact-project -f '*'/docker-compose.yml config --images core')
    [[ "${OMNI_TEST_CONFIG_STATUS}" == 0 ]] || exit "${OMNI_TEST_CONFIG_STATUS}"
    printf '%s' "${OMNI_TEST_CONFIG_REFS}"
    ;;
  'compose -p exact-project -f '*'/docker-compose.yml images -q core') printf '%s\n' "${OMNI_TEST_LEGACY_IMAGE}" ;;
  'image inspect --format {{.Id}} implicit-project-core'|'image inspect --format {{.Id}} registry.example/runtime:current')
    [[ -n "${OMNI_TEST_FRESH_IMAGE}" ]] || exit 1
    printf '%s\n' "${OMNI_TEST_FRESH_IMAGE}"
    ;;
  'image inspect --format {{.Id}} dependency:16') printf '%s\n' "${OMNI_TEST_DEPENDENCY_IMAGE}" ;;
  'image inspect --format {{.Id}} missing:latest') exit 1 ;;
  'image inspect --format {{.Id}} target-one:latest')
    [[ -n "${OMNI_TEST_ONE_IMAGE}" ]] || exit 1
    printf '%s\n' "${OMNI_TEST_ONE_IMAGE}"
    ;;
  'image inspect --format {{.Id}} target-two:latest')
    [[ -n "${OMNI_TEST_TWO_IMAGE}" ]] || exit 1
    printf '%s\n' "${OMNI_TEST_TWO_IMAGE}"
    ;;
  "image inspect --format {{ index .Config.Labels \"com.docker.compose.project\" }} ${OMNI_TEST_FRESH_IMAGE}") printf '%s\n' "${OMNI_TEST_FRESH_PROJECT}" ;;
  "image inspect --format {{ index .Config.Labels \"com.docker.compose.service\" }} ${OMNI_TEST_FRESH_IMAGE}") printf '%s\n' "${OMNI_TEST_FRESH_SERVICE}" ;;
  "image inspect --format {{ index .Config.Labels \"com.docker.compose.project\" }} ${OMNI_TEST_DEPENDENCY_IMAGE}") printf '%s\n' dependency-project ;;
  "image inspect --format {{ index .Config.Labels \"com.docker.compose.service\" }} ${OMNI_TEST_DEPENDENCY_IMAGE}") printf '%s\n' dependency ;;
  "image inspect --format {{ index .Config.Labels \"com.docker.compose.project\" }} ${OMNI_TEST_ONE_IMAGE}") printf '%s\n' "${OMNI_TEST_ONE_PROJECT}" ;;
  "image inspect --format {{ index .Config.Labels \"com.docker.compose.service\" }} ${OMNI_TEST_ONE_IMAGE}") printf '%s\n' "${OMNI_TEST_ONE_SERVICE}" ;;
  "image inspect --format {{ index .Config.Labels \"com.docker.compose.project\" }} ${OMNI_TEST_TWO_IMAGE}") printf '%s\n' "${OMNI_TEST_TWO_PROJECT}" ;;
  "image inspect --format {{ index .Config.Labels \"com.docker.compose.service\" }} ${OMNI_TEST_TWO_IMAGE}") printf '%s\n' "${OMNI_TEST_TWO_SERVICE}" ;;
  *) printf 'unexpected docker invocation: %s\n' "$*" >&2; exit 72 ;;
esac
`
	if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte(fakeDocker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_TEST_DOCKER_LOG", logPath)
	return fakeBin, logPath
}
