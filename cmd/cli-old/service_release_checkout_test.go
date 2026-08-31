package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/version"
)

func TestResolveServiceReleaseCheckoutAcceptsExactCleanHEAD(t *testing.T) {
	root, head := initializeServiceReleaseTestRepository(t)
	environment, err := serviceChildEnvironment(os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	checkout, err := resolveServiceReleaseCheckout(
		root, head, environment, execServiceProcessRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if checkout.Root != root || checkout.Commit != head {
		t.Fatalf("checkout = %+v, want root=%s commit=%s", checkout, root, head)
	}
}

func TestResolveServiceReleaseCheckoutRejectsMissingGitCheckout(t *testing.T) {
	root := t.TempDir()
	environment, err := serviceChildEnvironment(os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolveServiceReleaseCheckout(
		root, strings.Repeat("a", 40), environment, execServiceProcessRunner{},
	)
	if err == nil || !strings.Contains(err.Error(), "not an available Git checkout") {
		t.Fatalf("missing Git checkout error = %v", err)
	}
}

func TestResolveServiceReleaseCheckoutRejectsMissingGitExecutable(t *testing.T) {
	root := t.TempDir()
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)
	_, err := resolveServiceReleaseCheckout(
		root,
		strings.Repeat("a", 40),
		[]string{"PATH=" + emptyPath},
		execServiceProcessRunner{},
	)
	if err == nil || !strings.Contains(err.Error(), "not an available Git checkout") {
		t.Fatalf("missing Git executable error = %v", err)
	}
}

func TestResolveServiceReleaseCheckoutRejectsDirtyCheckout(t *testing.T) {
	root, head := initializeServiceReleaseTestRepository(t)
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	environment, err := serviceChildEnvironment(os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolveServiceReleaseCheckout(
		root, head, environment, execServiceProcessRunner{},
	)
	if err == nil || !strings.Contains(err.Error(), "checkout is dirty") {
		t.Fatalf("dirty checkout error = %v", err)
	}
}

func TestResolveServiceReleaseCheckoutRejectsLinkedWorktree(t *testing.T) {
	root, head := initializeServiceReleaseTestRepository(t)
	worktree := filepath.Join(t.TempDir(), "linked")
	runServiceReleaseTestGit(t, root, "worktree", "add", "--detach", worktree, head)
	t.Cleanup(func() {
		command := exec.Command("git", "-C", root, "worktree", "remove", "--force", worktree)
		_ = command.Run()
	})
	environment, err := serviceChildEnvironment(os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolveServiceReleaseCheckout(
		worktree, head, environment, execServiceProcessRunner{},
	)
	if err == nil || !strings.Contains(err.Error(), "real .git directory") {
		t.Fatalf("linked worktree error = %v", err)
	}
}

func TestResolveServiceReleaseCheckoutRejectsEmbeddedCommitMismatch(t *testing.T) {
	root, _ := initializeServiceReleaseTestRepository(t)
	environment, err := serviceChildEnvironment(os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolveServiceReleaseCheckout(
		root, strings.Repeat("f", 40), environment, execServiceProcessRunner{},
	)
	if err == nil || !strings.Contains(err.Error(), "does not equal embedded release commit") {
		t.Fatalf("commit mismatch error = %v", err)
	}
}

func TestResolveServiceReleaseCheckoutRejectsPaddedEmbeddedCommit(t *testing.T) {
	root, head := initializeServiceReleaseTestRepository(t)
	environment, err := serviceChildEnvironment(os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolveServiceReleaseCheckout(
		root, head+"\n", environment, execServiceProcessRunner{},
	)
	if err == nil || !strings.Contains(err.Error(), "embedded release commit") {
		t.Fatalf("padded embedded commit error = %v", err)
	}
}

func TestServiceChildEnvironmentOverwritesOrRemovesAmbientCommit(t *testing.T) {
	expected := strings.Repeat("a", 40)
	base := []string{"PATH=/bin", "OMNIDEX_COMMIT=ambient", "OTHER=value"}
	bound, err := serviceChildEnvironment(base, expected)
	if err != nil {
		t.Fatal(err)
	}
	if got := serviceEnvironmentValues(bound, serviceReleaseCommitEnvironmentKey); len(got) != 1 || got[0] != expected {
		t.Fatalf("bound release commit values = %v", got)
	}
	clean, err := serviceChildEnvironment(base, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := serviceEnvironmentValues(clean, serviceReleaseCommitEnvironmentKey); len(got) != 0 {
		t.Fatalf("sanitized release commit values = %v", got)
	}
	if _, err := serviceChildEnvironment(base, " "+expected); err == nil {
		t.Fatal("expected padded release commit to fail")
	}
}

func TestPrepareServiceReleaseEnvironmentBindsCoreStartAndRestart(t *testing.T) {
	root, head := initializeServiceReleaseTestRepository(t)
	original := version.Commit
	t.Cleanup(func() { version.Commit = original })
	version.Commit = head

	for _, opts := range []serviceCommandOptions{
		{Service: "core", Action: "start"},
		{Service: "all", Action: "restart"},
	} {
		releaseCommit, environment, err := prepareServiceReleaseEnvironment(
			opts, root, []string{"PATH=" + os.Getenv("PATH")}, execServiceProcessRunner{},
		)
		if err != nil {
			t.Fatalf("prepare %s %s release environment: %v", opts.Action, opts.Service, err)
		}
		if releaseCommit != head {
			t.Fatalf("%s %s release commit = %q, want %q", opts.Action, opts.Service, releaseCommit, head)
		}
		if values := serviceEnvironmentValues(environment, serviceReleaseCommitEnvironmentKey); len(values) != 1 || values[0] != head {
			t.Fatalf("%s %s child release values = %v", opts.Action, opts.Service, values)
		}
	}
}

func TestServiceDeploymentChildEnvironmentOverwritesAmbientHostIdentity(t *testing.T) {
	commit := strings.Repeat("a", 40)
	base := []string{
		"PATH=/bin",
		"OMNIDEX_COMMIT=ambient",
		"HOST_UID=9000",
		"HOST_UID=9002",
		"HOST_GID=9001",
		"COMPOSE_PROJECT_NAME=ambient-project",
		"COMPOSE_PROJECT_NAME=second-ambient-project",
		"DOCKER_CONTEXT=rootless",
		"DOCKER_HOST=unix:///run/user/1000/docker.sock",
		"DOCKER_CONFIG=/tmp/alternate-docker-config",
		"DOCKER_TLS_VERIFY=1",
		"DOCKER_TLS=1",
		"DOCKER_CERT_PATH=/tmp/alternate-docker-certs",
		"BUILDX_BUILDER=alternate-builder",
		"BUILDX_CONFIG=/tmp/alternate-buildx-config",
		"BUILDKIT_HOST=tcp://127.0.0.1:1234",
		"BUILDKIT_TLS_SERVER_NAME=alternate",
		"BUILDKIT_TLS_CACERT=/tmp/alternate-buildkit-ca",
		"BUILDKIT_TLS_CERT=/tmp/alternate-buildkit-cert",
		"BUILDKIT_TLS_KEY=/tmp/alternate-buildkit-key",
	}
	bound, err := serviceDeploymentChildEnvironment(base, commit, "omnidex", "1000", "1001")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		dockerContextEnvironmentKey, dockerHostEnvironmentKey, "DOCKER_CONFIG",
		"DOCKER_CERT_PATH", "DOCKER_TLS", "DOCKER_TLS_VERIFY",
		"BUILDKIT_HOST", "BUILDKIT_TLS_SERVER_NAME", "BUILDKIT_TLS_CACERT",
		"BUILDKIT_TLS_CERT", "BUILDKIT_TLS_KEY", "BUILDX_BUILDER", "BUILDX_CONFIG",
	} {
		if values := serviceEnvironmentValues(bound, key); len(values) != 0 {
			t.Fatalf("bound environment retained ambient %s values = %v", key, values)
		}
	}
	for key, expected := range map[string]string{
		serviceReleaseCommitEnvironmentKey: commit,
		hostUIDEnvironmentKey:              "1000",
		hostGIDEnvironmentKey:              "1001",
		composeProjectEnvironmentKey:       "omnidex",
	} {
		if values := serviceEnvironmentValues(bound, key); len(values) != 1 || values[0] != expected {
			t.Fatalf("bound %s values = %v", key, values)
		}
	}
	if _, err := serviceDeploymentChildEnvironment(base, commit, "omnidex", "01000", "1001"); err == nil {
		t.Fatal("expected noncanonical host identity to fail")
	}
	if _, err := serviceDeploymentChildEnvironment(base, commit, "bad/project", "1000", "1001"); err == nil {
		t.Fatal("expected invalid Compose project identity to fail")
	}
}

func initializeServiceReleaseTestRepository(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for checkout identity tests")
	}
	root := t.TempDir()
	runServiceReleaseTestGit(t, root, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runServiceReleaseTestGit(t, root, "add", "tracked.txt")
	runServiceReleaseTestGit(
		t, root,
		"-c", "user.name=Omnidex Test", "-c", "user.email=test@omnidex.invalid",
		"commit", "--quiet", "-m", "release identity fixture",
	)
	command := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return root, strings.TrimSuffix(string(output), "\n")
}

func runServiceReleaseTestGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func serviceEnvironmentValues(environment []string, key string) []string {
	values := []string{}
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found && name == key {
			values = append(values, value)
		}
	}
	return values
}
