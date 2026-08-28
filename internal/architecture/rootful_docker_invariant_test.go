package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionDockerAuthorityHasNoRootlessSelector(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	paths := []string{
		"docker-compose.yml", "default.env", ".env.example", "up.sh", "down.sh",
		"update.sh", "scripts/compose-deployment.sh", "scripts/update-runtime-lib.sh",
		"scripts/recover-worknet-omni.sh", "scripts/setup-host-deps.sh",
		"scripts/setup-host-deps.ps1", "cmd/cli/service_deployment_identity.go",
		"cmd/cli/service_compose.go", "cmd/cli/service_process.go",
		"internal/worker/v3_command_docker_runtime.go",
		"internal/worker/v3_project_environment_docker.go",
	}
	for _, name := range paths {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, forbidden := range []string{
			"DOCKER_CONTEXT=rootless", "DOCKER_CONTEXT_NAME=\"rootless\"",
			"/run/user/", "dockerd-rootless", "${DOCKER_SOCKET_PATH",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s retains forbidden rootless Docker selector %q", name, forbidden)
			}
		}
	}
}

func TestRootfulDockerAuthorityIsExplicitAndDocumented(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	for name, required := range map[string][]string{
		"docker-compose.yml": {
			"source: /var/run/docker.sock", "target: /var/run/docker.sock",
			"DOCKER_HOST: unix:///var/run/docker.sock",
		},
		"scripts/update-runtime-lib.sh": {
			"runtime_require_rootful_docker_context", "DOCKER_CONTEXT=default",
			"-u DOCKER_HOST", "-u DOCKER_CONFIG",
		},
		"docs/ROOTFUL_DOCKER.md": {
			"Rootful Docker Is an Omnidex Invariant", "/var/run/docker.sock",
			"Rootless Docker must never be used",
		},
	} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, fragment := range required {
			if !strings.Contains(string(raw), fragment) {
				t.Errorf("%s omits rootful Docker authority %q", name, fragment)
			}
		}
	}
}
