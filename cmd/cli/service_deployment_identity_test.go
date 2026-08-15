package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceDeploymentIdentityUsesExactManagedEnvironment(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=omni-nxt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := readServiceDeploymentIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if identity.DockerContext != "rootless" || identity.ComposeProject != "omni-nxt" {
		t.Fatalf("deployment identity = %+v", identity)
	}
	if got := strings.Join(identity.composeCommandPrefix(), " "); got != "docker --context rootless compose -p omni-nxt" {
		t.Fatalf("Compose prefix = %q", got)
	}
	if got := strings.Join(identity.dockerCommandPrefix(), " "); got != "docker --context rootless" {
		t.Fatalf("Docker prefix = %q", got)
	}
}

func TestServiceDeploymentIdentityRejectsMissingBlankDuplicateAndInvalidAuthority(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "missing context", raw: "COMPOSE_PROJECT_NAME=omni-nxt\n", want: "does not define DOCKER_CONTEXT"},
		{name: "blank context", raw: "DOCKER_CONTEXT=\nCOMPOSE_PROJECT_NAME=omni-nxt\n", want: "DOCKER_CONTEXT must be explicit"},
		{name: "duplicate context", raw: "DOCKER_CONTEXT=rootless\nDOCKER_CONTEXT=default\nCOMPOSE_PROJECT_NAME=omni-nxt\n", want: "defined more than once"},
		{name: "missing project", raw: "DOCKER_CONTEXT=rootless\n", want: "does not define COMPOSE_PROJECT_NAME"},
		{name: "blank project", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=\n", want: "COMPOSE_PROJECT_NAME must be explicit"},
		{name: "invalid project", raw: "DOCKER_CONTEXT=rootless\nCOMPOSE_PROJECT_NAME=bad/project\n", want: "unsupported characters"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ".env"), []byte(test.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := readServiceDeploymentIdentity(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("deployment identity error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestServiceRuntimeRootsPreferTheInvokedBinaryOverAmbientInstallState(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "authoritative", "bin", "agent-cli")
	got := serviceRuntimeRootCandidates(
		filepath.Join(root, "stale-install"),
		filepath.Join(root, "working-directory"),
		executable,
	)
	wantFirst := filepath.Join(root, "authoritative", "bin")
	wantSecond := filepath.Join(root, "authoritative")
	if len(got) < 2 || got[0] != wantFirst || got[1] != wantSecond {
		t.Fatalf("service runtime roots = %v, want invoked binary roots first", got)
	}
}
