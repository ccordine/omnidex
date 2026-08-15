package omni

import (
	"strings"
	"testing"
)

func TestDockerRuntimeUsesTheExactBuilderGoToolchain(t *testing.T) {
	root := repoRootFromOmniTest(t)
	dockerfile := readRepoScript(t, root, "Dockerfile")
	compose := readRepoScript(t, root, "docker-compose.yml")
	if !strings.Contains(dockerfile, "COPY --from=build /usr/local/go /usr/local/go") {
		t.Fatal("Docker runtime must copy the exact builder Go toolchain")
	}
	if strings.Contains(dockerfile, "apk add --no-cache git go nodejs npm") {
		t.Fatal("Docker runtime must not replace the builder Go toolchain with the Alpine package")
	}
	if !strings.Contains(dockerfile, "ENV PATH=/usr/local/go/bin:") {
		t.Fatal("Docker runtime PATH does not select the exact builder Go toolchain")
	}
	if !strings.Contains(compose, "PATH: /usr/local/go/bin:") {
		t.Fatal("Compose must not hide the exact runtime Go toolchain")
	}
}
