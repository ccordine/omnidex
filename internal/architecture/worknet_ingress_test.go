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
	if !strings.Contains(compose, `CORE_URL: ${CORE_URL:-https://omni.worknet}`) {
		t.Fatal("core must advertise the canonical WorkNet HTTPS URL")
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
	if !strings.Contains(raw, `${HOST_WORKSPACE_PATH:?HOST_WORKSPACE_PATH must be set to an absolute project root}:/workspace:rw`) {
		t.Fatal("core must mount exactly the configured host project root read-write")
	}
	if strings.Contains(raw, ":/workspace:ro") {
		t.Fatal("read-only workspace mount makes native coding incapable of writing")
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
	if !strings.Contains(image, "build-base") || !strings.Contains(image, "CGO_ENABLED=1") {
		t.Fatal("runtime core build must compile the authoritative tree-sitter TypeScript parser with CGO")
	}
}
