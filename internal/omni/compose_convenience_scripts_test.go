package omni

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeConvenienceScriptsUseConfiguredDeploymentIdentity(t *testing.T) {
	root := repoRootFromOmniTest(t)
	for _, scriptName := range []string{"up.sh", "down.sh"} {
		body := readRepoScript(t, root, scriptName)
		if strings.Contains(body, "docker compose") {
			t.Fatalf("%s bypasses configured Docker deployment identity", scriptName)
		}
		if !strings.Contains(body, "scripts/compose-deployment.sh") {
			t.Fatalf("%s does not delegate to the configured deployment wrapper", scriptName)
		}
	}

	body := readRepoScript(t, root, "scripts/compose-deployment.sh")
	for _, required := range []string{
		"DOCKER_CONTEXT", "COMPOSE_PROJECT_NAME", "HOST_UID", "HOST_GID", "managed_checkout_require_env_key",
		"managed_checkout_env_value", "compose_command_array", "--remove-orphans",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("compose deployment wrapper omits %q", required)
		}
	}
	if strings.Contains(body, "down -v") || strings.Contains(body, "--volumes") {
		t.Fatal("compose deployment wrapper must not remove durable volumes")
	}
	up := readRepoScript(t, root, "up.sh")
	for _, required := range []string{
		"host service start", "ollama.service", "scripts/compose-deployment.sh", "core:status",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("up.sh omits required dependency orchestration %q", required)
		}
	}
}

func TestComposeDeploymentWrapperUsesConfiguredDockerContextAndProject(t *testing.T) {
	root := repoRootFromOmniTest(t)
	stage := t.TempDir()
	for _, name := range []string{
		"scripts/compose-deployment.sh",
		"scripts/managed-checkout-lib.sh",
		"scripts/update-runtime-lib.sh",
	} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(stage, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFixtureFile(t, filepath.Join(stage, ".gitignore"), ".env\n", 0o600)
	writeFixtureFile(t, filepath.Join(stage, ".env"), "DOCKER_CONTEXT=default\nCOMPOSE_PROJECT_NAME=exact-project\nHOST_UID=1000\nHOST_GID=1001\n", 0o600)
	writeFixtureFile(t, filepath.Join(stage, "docker-compose.yml"), "services: {}\n", 0o600)
	runFixtureGit(t, stage, "init", "-b", "main")
	runFixtureGit(t, stage, "config", "user.email", "compose-test@example.invalid")
	runFixtureGit(t, stage, "config", "user.name", "Compose Test")
	runFixtureGit(t, stage, "add", ".")
	runFixtureGit(t, stage, "commit", "-m", "fixture")
	commit := runFixtureGit(t, stage, "rev-parse", "HEAD")

	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	fakeDocker := filepath.Join(bin, "docker")
	fakeDockerBody := `#!/usr/bin/env bash
set -euo pipefail
printf '%s|%s|%s|%s|%s|%s\n' "${OMNIDEX_COMMIT:-}" "$DOCKER_CONTEXT" "${COMPOSE_PROJECT_NAME:-}" "${HOST_UID:-}" "${HOST_GID:-}" "$*" >>"$FAKE_DOCKER_LOG"
case "$*" in
  "compose version") exit 0 ;;
  *" build --pull core"|*" up -d --remove-orphans --wait --wait-timeout 180 core") exit 0 ;;
  *" images -q core") printf '%s\n' "$FAKE_EXPECTED_IMAGE" ;;
  *" ps -q core") printf '%s\n' "$FAKE_CONTAINER_ID" ;;
	  "image inspect --format {{ index .Config.Labels \"org.opencontainers.image.revision\" }} "*) printf '%s\n' "$FAKE_EXPECTED_COMMIT" ;;
	  "image inspect --format {{.Config.User}} "*) printf '%s\n' "$FAKE_EXPECTED_USER" ;;
	  "inspect --type container --format {{.Image}} "*) printf '%s\n' "$FAKE_EXPECTED_IMAGE" ;;
	  "inspect --type container --format {{ index .Config.Labels \"org.opencontainers.image.revision\" }} "*) printf '%s\n' "$FAKE_EXPECTED_COMMIT" ;;
	  "inspect --type container --format {{.Config.User}} "*) printf '%s\n' "$FAKE_EXPECTED_USER" ;;
  "exec "*" /usr/local/bin/agent-core release:verify-running-health "*) printf '%s\n' "$FAKE_EXPECTED_COMMIT" ;;
  "exec "*" sh -ec "*) exit 0 ;;
  *) printf 'unexpected docker invocation: %s\n' "$*" >&2; exit 72 ;;
esac
`
	if err := os.WriteFile(fakeDocker, []byte(fakeDockerBody), 0o755); err != nil {
		t.Fatal(err)
	}

	expectedImage := "sha256:" + strings.Repeat("a", 64)
	containerID := strings.Repeat("b", 64)
	command := exec.Command("bash", filepath.Join(stage, "scripts/compose-deployment.sh"), "up", "--build")
	command.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"FAKE_DOCKER_LOG="+logPath,
		"FAKE_EXPECTED_IMAGE="+expectedImage,
		"FAKE_CONTAINER_ID="+containerID,
		"FAKE_EXPECTED_COMMIT="+commit,
		"FAKE_EXPECTED_USER=1000:1001",
		"OMNIDEX_COMMIT="+strings.Repeat("f", 40),
		"HOST_UID=9000",
		"HOST_GID=9001",
		"COMPOSE_PROJECT_NAME=ambient-project",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("configured compose wrapper failed: %v\n%s", err, output)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, commit+"|default|exact-project|1000|1001|") {
			t.Fatalf("wrapper did not overwrite ambient build or host identity: %q", line)
		}
	}
	ordered := []string{
		"compose version",
		"compose -p exact-project -f " + filepath.Join(stage, "docker-compose.yml") + " build --pull core",
		"compose -p exact-project -f " + filepath.Join(stage, "docker-compose.yml") + " images -q core",
		"image inspect --format {{ index .Config.Labels \"org.opencontainers.image.revision\" }} " + expectedImage,
		"image inspect --format {{.Config.User}} " + expectedImage,
		"compose -p exact-project -f " + filepath.Join(stage, "docker-compose.yml") + " up -d --remove-orphans --wait --wait-timeout 180 core",
		"inspect --type container --format {{.Image}} " + containerID,
		"inspect --type container --format {{ index .Config.Labels \"org.opencontainers.image.revision\" }} " + containerID,
		"inspect --type container --format {{.Config.User}} " + containerID,
		"exec " + containerID + " /usr/local/bin/agent-core release:verify-running-health " + commit,
		"exec " + containerID + " sh -ec",
	}
	position := -1
	rawLog := string(log)
	for _, fragment := range ordered {
		next := strings.Index(rawLog[position+1:], fragment)
		if next < 0 {
			t.Fatalf("wrapper Docker log omits ordered operation %q:\n%s", fragment, rawLog)
		}
		position += next + 1
	}
}

func TestComposeDeploymentDownDoesNotRequireBuildCheckoutIdentity(t *testing.T) {
	root := repoRootFromOmniTest(t)
	stage := t.TempDir()
	for _, name := range []string{
		"scripts/compose-deployment.sh",
		"scripts/managed-checkout-lib.sh",
		"scripts/update-runtime-lib.sh",
	} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, filepath.Join(stage, name), string(data), 0o755)
	}
	writeFixtureFile(t, filepath.Join(stage, ".env"), "DOCKER_CONTEXT=default\nCOMPOSE_PROJECT_NAME=exact-project\nHOST_UID=1000\nHOST_GID=1001\n", 0o600)
	writeFixtureFile(t, filepath.Join(stage, "docker-compose.yml"), "services: {}\n", 0o600)

	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker.log")
	writeFixtureFile(t, filepath.Join(bin, "docker"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s|%s|%s|%s|%s\n' "${DOCKER_CONTEXT:-}" "${COMPOSE_PROJECT_NAME:-}" "${HOST_UID:-}" "${HOST_GID:-}" "$*" >>"$FAKE_DOCKER_LOG"
case "$*" in
  "compose version"|*" down --remove-orphans") exit 0 ;;
  *) exit 72 ;;
esac
`, 0o755)

	command := exec.Command("bash", filepath.Join(stage, "scripts/compose-deployment.sh"), "down")
	command.Env = exactTestEnvironment(os.Environ(), map[string]string{
		"PATH":                 bin + ":" + os.Getenv("PATH"),
		"FAKE_DOCKER_LOG":      logPath,
		"OMNIDEX_COMMIT":       "invalid-ambient-value",
		"HOST_UID":             "9000",
		"HOST_GID":             "9001",
		"COMPOSE_PROJECT_NAME": "ambient-project",
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("non-build down failed without Git identity: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	if !strings.Contains(log, "default|exact-project|1000|1001|compose version") ||
		!strings.Contains(log, "default|exact-project|1000|1001|compose -p exact-project -f "+filepath.Join(stage, "docker-compose.yml")+" down --remove-orphans") {
		t.Fatalf("down Docker log=%q", log)
	}
	if strings.Contains(log, " build ") || strings.Contains(log, " up ") {
		t.Fatalf("down invoked build/runtime work: %q", log)
	}
}

func TestOperatorGuidanceDoesNotSendUsersThroughAmbientCompose(t *testing.T) {
	root := repoRootFromOmniTest(t)
	for _, path := range []string{"scripts/ufw-docker-host.sh", "internal/api/host_status.go"} {
		body := readRepoScript(t, root, path)
		for _, forbidden := range []string{"docker compose up", "docker compose exec"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s retains ambient deployment guidance %q", path, forbidden)
			}
		}
	}
}
