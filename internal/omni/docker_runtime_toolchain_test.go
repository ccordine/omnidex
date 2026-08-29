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
	if !strings.Contains(dockerfile, "ENV GOROOT=/usr/local/go") {
		t.Fatal("Docker runtime must expose the copied toolchain root to trimpath-built binaries")
	}
	if !strings.Contains(compose, "PATH: /usr/local/go/bin:") {
		t.Fatal("Compose must not hide the exact runtime Go toolchain")
	}
	if !strings.Contains(compose, "GOROOT: /usr/local/go") {
		t.Fatal("Compose must preserve the exact runtime Go toolchain root")
	}
}

func TestDockerRuntimeInstallsExistingRepositorySandbox(t *testing.T) {
	root := repoRootFromOmniTest(t)
	dockerfile := readRepoScript(t, root, "Dockerfile")
	compose := readRepoScript(t, root, "docker-compose.yml")
	if !strings.Contains(dockerfile, "apk add --no-cache git nodejs npm bubblewrap build-base") {
		t.Fatal("Docker runtime must install Bubblewrap and its required Go build paths")
	}
	if !strings.Contains(compose, "- seccomp=unconfined") {
		t.Fatal("Compose must permit Bubblewrap to create its nested user namespace")
	}
	if !strings.Contains(compose, "- systempaths=unconfined") {
		t.Fatal("Compose must permit Bubblewrap to mount its isolated proc filesystem")
	}
	if !strings.Contains(compose, "GOMODCACHE: /var/cache/omnidex/gomod") ||
		!strings.Contains(compose, "source: gomodcache\n        target: /var/cache/omnidex/gomod") ||
		!strings.Contains(compose, "gomodcache:") {
		t.Fatal("Compose must provide one explicit persistent code-owned Go module cache")
	}
}
