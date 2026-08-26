package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerComposeUsesWorkNetAsTheOnlyPublicCoreIngress(t *testing.T) {
	composePath := filepath.Clean(filepath.Join("..", "..", "docker-compose.yml"))
	raw, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)

	if strings.Contains(compose, `"8090:8090"`) {
		t.Fatal("core must not claim host port 8090; WorkNet owns public ingress")
	}
	if !strings.Contains(compose, `CORE_URL: ${CORE_URL:?CORE_URL must be configured}`) {
		t.Fatal("core must require the one explicitly configured public ingress URL")
	}
	if strings.Contains(compose, `CORE_URL: ${CORE_URL:-`) {
		t.Fatal("core must not silently fall back to a different public ingress URL")
	}
	if !strings.Contains(compose, "    expose:\n      - \"8090\"") {
		t.Fatal("core must expose port 8090 to Docker networks")
	}
	if !strings.Contains(compose, "      - subnet: 172.16.90.0/24") {
		t.Fatal("Omnidex edge must stay inside WorkNet's Docker-to-Ollama trust boundary")
	}

	for _, name := range []string{".env.example", "default.env"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "CORE_URL=https://omni.worknet") {
			t.Fatalf("%s must advertise the canonical WorkNet HTTPS URL", name)
		}
	}
}

func TestDockerRuntimeCanBuildInsideOneExplicitWorkspaceBoundary(t *testing.T) {
	compose, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "docker-compose.yml")))
	if err != nil {
		t.Fatal(err)
	}
	raw := string(compose)
	if !strings.Contains(raw, "source: ${HOST_WORKSPACE_PATH:?HOST_WORKSPACE_PATH must be set to an absolute project root}\n        target: /workspace") {
		t.Fatal("core must mount exactly the configured host project root read-write")
	}
	for _, required := range []string{
		`user: "0:0"`,
		`DOCKER_HOST: unix:///var/run/docker.sock`,
		"source: ${HOST_WORKSPACE_PATH:?HOST_WORKSPACE_PATH must be set to an absolute project root}\n        target: ${HOST_WORKSPACE_PATH:?HOST_WORKSPACE_PATH must be set to an absolute project root}",
		"source: ${DOCKER_SOCKET_PATH:?DOCKER_SOCKET_PATH must name the rootless Docker Unix socket}\n        target: /var/run/docker.sock",
	} {
		if !strings.Contains(raw, required) {
			t.Fatalf("core Docker execution boundary lacks %q", required)
		}
	}
	if strings.Count(raw, "create_host_path: false") < 3 {
		t.Fatal("Docker workspace and socket binds must fail instead of creating missing host paths")
	}

	dockerfile, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "Dockerfile")))
	if err != nil {
		t.Fatal(err)
	}
	image := string(dockerfile)
	for _, tool := range []string{"go", "nodejs", "npm", "git"} {
		if !strings.Contains(image, tool) {
			t.Fatalf("runtime image does not declare required build tool %q", tool)
		}
	}
	for _, authority := range []string{
		"golang:1.24.1-alpine@sha256:43c094ad24b6ac0546c62193baeb3e6e49ce14d3250845d166c77c25f64b0386",
		"docker:29.5.1-cli@sha256:b40b3737eb3bf588d25bb856d3564dd3f9fdb32ac2fc19ebe85cc58d761692a5",
		"docker/compose-bin:v5.1.4@sha256:88d82497d9be33710c959aeaad5e541de5aa41a36d733e04ab09ccce74fa6b4c",
		"/usr/local/libexec/docker/cli-plugins/docker-compose",
		"Docker version 29\\.5\\.1,",
		`docker compose version --short)" = "5.1.4"`,
		"ENV DOCKER_COMPOSE_VERSION=5.1.4",
		"ENTRYPOINT []",
	} {
		if !strings.Contains(image, authority) {
			t.Fatalf("runtime image lacks exact Docker toolchain authority %q", authority)
		}
	}
	for _, legacy := range []string{
		"FROM golang:1.24.1-alpine AS build",
		"apk add --no-cache docker-cli",
		"docker-cli-compose",
	} {
		if strings.Contains(image, legacy) {
			t.Fatalf("runtime image retains unpinned Docker package fallback %q", legacy)
		}
	}
	if !strings.Contains(image, "build-base") || !strings.Contains(image, "CGO_ENABLED=1") {
		t.Fatal("runtime core build must compile the authoritative tree-sitter TypeScript parser with CGO")
	}
	if !strings.Contains(image, "USER app") {
		t.Fatal("standalone core image must retain its non-root default identity")
	}
	for _, name := range []string{".env.example", "default.env"} {
		environment, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", name)))
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{
			"HOST_UID=1000", "HOST_GID=1000", "DOCKER_SOCKET_PATH=/run/user/1000/docker.sock",
		} {
			if !strings.Contains(string(environment), required) {
				t.Fatalf("%s lacks rootless Docker identity authority %q", name, required)
			}
		}
	}
	runner, err := os.ReadFile(filepath.Clean(filepath.Join("..", "worker", "v3_command_execution.go")))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"resolveV3CommandExecution(ctx, root, program)",
		"process.Dir = executionRoot",
		"environment = append(environment, commandEnvironment...)",
	} {
		if !strings.Contains(string(runner), required) {
			t.Fatalf("command runner does not consume Docker execution boundary %q", required)
		}
	}
	dockerBoundary, err := os.ReadFile(filepath.Clean(filepath.Join("..", "worker", "v3_command_docker_runtime.go")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerBoundary), "os.SameFile(runtimeInfo, hostInfo)") {
		t.Fatal("Docker command boundary does not prove runtime/host mount identity")
	}
	if !strings.Contains(string(dockerBoundary), "validateV3RootlessDockerDaemon") {
		t.Fatal("Docker command boundary does not live-qualify the daemon as rootless")
	}
}
