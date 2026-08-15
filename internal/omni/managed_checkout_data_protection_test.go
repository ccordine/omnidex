package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedInstallAcceptsAnExistingEmptyTarget(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	if err := os.Mkdir(fixture.prefix, 0o755); err != nil {
		t.Fatal(err)
	}
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", managedCheckoutTestEnvironment(t), "--skip-deps", "--yes")
	assertManagedCheckoutClean(t, fixture.prefix)
}

func TestManagedUpdateRejectsUntrackedInstallDataWithoutDeletingIt(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	environment := managedCheckoutTestEnvironment(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", environment, "--skip-deps", "--yes")
	oldHead := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD")
	dataPath := filepath.Join(fixture.prefix, "operator-data.txt")
	writeFixtureFile(t, dataPath, "must survive exactly\n", 0o600)

	output, err := runManagedScriptResult(fixture, filepath.Join(fixture.prefix, "update.sh"), []string{"OMNI_FIXTURE_NPM_FAIL=1"},
		"--host-only", "--no-host-restart", "--no-pull")
	if err == nil || !strings.Contains(output, "untracked files") {
		t.Fatalf("untracked install update error=%v output=%s", err, output)
	}
	if got := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD"); got != oldHead {
		t.Fatalf("rejected update changed active HEAD: got=%s want=%s", got, oldHead)
	}
	if got := readFixtureFile(t, dataPath); got != "must survive exactly\n" {
		t.Fatalf("rejected update changed untracked data: %q", got)
	}
}

func TestManagedUpdateRejectsIgnoredRuntimeDataWithoutDeletingIt(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	environment := managedCheckoutTestEnvironment(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", environment, "--skip-deps", "--yes")
	oldHead := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD")
	dataPath := filepath.Join(fixture.prefix, "logs", "operator.log")
	writeFixtureFile(t, dataPath, "must also survive exactly\n", 0o600)

	output, err := runManagedScriptResult(fixture, filepath.Join(fixture.prefix, "update.sh"), []string{"OMNI_FIXTURE_NPM_FAIL=1"},
		"--host-only", "--no-host-restart", "--no-pull")
	if err == nil || !strings.Contains(output, "publication would remove: logs/operator.log") {
		t.Fatalf("ignored runtime-data update error=%v output=%s", err, output)
	}
	if got := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD"); got != oldHead {
		t.Fatalf("rejected update changed active HEAD: got=%s want=%s", got, oldHead)
	}
	if got := readFixtureFile(t, dataPath); got != "must also survive exactly\n" {
		t.Fatalf("rejected update changed ignored runtime data: %q", got)
	}
}

func TestManagedReinstallRejectsIgnoredRuntimeDataWithoutDeletingIt(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	environment := managedCheckoutTestEnvironment(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", environment, "--skip-deps", "--yes")
	dataPath := filepath.Join(fixture.prefix, ".omni", "runtime.json")
	writeFixtureFile(t, dataPath, "{\"preserve\":true}\n", 0o600)

	output, err := runManagedScriptResult(fixture, filepath.Join(fixture.source, "install.sh"), []string{"OMNI_FIXTURE_NPM_FAIL=1"},
		"--prefix", fixture.prefix, "--skip-deps", "--yes")
	if err == nil || !strings.Contains(output, "publication would remove: .omni/runtime.json") {
		t.Fatalf("ignored reinstall data error=%v output=%s", err, output)
	}
	if got := readFixtureFile(t, dataPath); got != "{\"preserve\":true}\n" {
		t.Fatalf("rejected reinstall changed ignored runtime data: %q", got)
	}
}
