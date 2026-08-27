package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeInitializesOnlyPrivateRuntimeVolumesBeforeNonRootCore(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	composeBytes, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	compose := string(composeBytes)
	for _, required := range []string{
		"runtime-volume-init:\n        condition: service_completed_successfully",
		"  runtime-volume-init:\n",
		`image: ${COMPOSE_PROJECT_NAME:?COMPOSE_PROJECT_NAME must be configured}-core`,
		`user: "0:0"`,
		"network_mode: none",
		"read_only: true",
		"no-new-privileges:true",
		"/usr/local/bin/initialize-omnidex-volumes",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("runtime volume initialization omits %q", required)
		}
	}
	initSection := strings.SplitN(compose, "\n  runtime-volume-init:\n", 2)[1]
	initSection = strings.SplitN(initSection, "\n  postgres:\n", 2)[0]
	for _, forbidden := range []string{
		"source: ${HOST_WORKSPACE_PATH", "source: ${DOCKER_SOCKET_PATH", "target: /workspace",
	} {
		if strings.Contains(initSection, forbidden) {
			t.Fatalf("root volume initializer receives forbidden host authority %q", forbidden)
		}
	}
	for _, volume := range []string{"source: deploymentkeys", "source: gomodcache"} {
		if strings.Count(initSection, volume) != 1 {
			t.Fatalf("root volume initializer must receive exactly one %q mount", volume)
		}
	}

	dockerfileBytes, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(dockerfileBytes)
	for _, required := range []string{
		"COPY scripts/initialize-runtime-volumes.sh /usr/local/bin/initialize-omnidex-volumes",
		"mkdir -p /var/lib/omnidex-deployment /var/cache/omnidex/gomod",
		"chown -R app:app /var/lib/omnidex-deployment /var/cache/omnidex/gomod",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Fatalf("runtime image volume ownership contract omits %q", required)
		}
	}
	if strings.Contains(compose, "nocopy: true") {
		t.Fatal("fresh runtime volumes still suppress image-owned directory initialization")
	}
}
