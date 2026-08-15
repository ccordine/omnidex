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
		"DOCKER_CONTEXT", "COMPOSE_PROJECT_NAME", "managed_checkout_require_env_key",
		"managed_checkout_env_value", "compose_command_array", "--remove-orphans",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("compose deployment wrapper omits %q", required)
		}
	}
	if strings.Contains(body, "down -v") || strings.Contains(body, "--volumes") {
		t.Fatal("compose deployment wrapper must not remove durable volumes")
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
	if err := os.WriteFile(filepath.Join(stage, ".env"), []byte("DOCKER_CONTEXT=default\nCOMPOSE_PROJECT_NAME=exact-project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "docker-compose.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(stage, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(stage, "docker.log")
	fakeDocker := filepath.Join(bin, "docker")
	if err := os.WriteFile(fakeDocker, []byte("#!/usr/bin/env bash\nprintf '%s|%s\\n' \"$DOCKER_CONTEXT\" \"$*\" >>\"$FAKE_DOCKER_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("bash", filepath.Join(stage, "scripts/compose-deployment.sh"), "up", "--build")
	command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "FAKE_DOCKER_LOG="+logPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("configured compose wrapper failed: %v\n%s", err, output)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 2 || lines[0] != "default|compose version" ||
		!strings.Contains(lines[1], "default|compose -p exact-project -f "+filepath.Join(stage, "docker-compose.yml")+" up -d --remove-orphans --build") {
		t.Fatalf("wrapper docker calls=%q", lines)
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
