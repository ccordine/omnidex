package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseBuilderPublishesNothingWhenTrackedSourceChangesAfterPreflight(t *testing.T) {
	fixture := newReleaseStageFixture(t)
	writeReleaseTestFile(t, filepath.Join(fixture.repository, "tracked.txt"), "changed during build\n")
	output, err := runReleasePublish(t, fixture, `verify_source_stage() { :; }`)
	if err == nil || !strings.Contains(output, "tracked source changed during release build") {
		t.Fatalf("source mutation error=%v output=%q", err, output)
	}
	assertNoReleasePublication(t, fixture.dist)
}

func TestReleaseBuilderRejectsMutatedImmutableSourceStageBeforePublication(t *testing.T) {
	fixture := newReleaseStageFixture(t)
	writeReleaseTestFile(t, filepath.Join(fixture.source, "source.txt"), "tampered\n")
	output, err := runReleasePublish(t, fixture, "")
	if err == nil || !strings.Contains(output, "immutable source") {
		t.Fatalf("stage mutation error=%v output=%q", err, output)
	}
	assertNoReleasePublication(t, fixture.dist)
}

func TestReleaseBuilderAtomicPublicationLeavesNothingWhenRenameFails(t *testing.T) {
	fixture := newReleaseStageFixture(t)
	fakeBin := t.TempDir()
	fakeMove := filepath.Join(fakeBin, "mv")
	if err := os.WriteFile(fakeMove, []byte("#!/usr/bin/env bash\necho forced atomic rename failure >&2\nexit 71\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := runReleasePublish(t, fixture, `PATH="`+fakeBin+`:${PATH}"`)
	if err == nil || !strings.Contains(output, "forced atomic rename failure") {
		t.Fatalf("atomic publish error=%v output=%q", err, output)
	}
	assertNoReleasePublication(t, fixture.dist)
}

type releaseStageFixture struct {
	repository, dist, commit, stage, source, output, manifest string
}

func newReleaseStageFixture(t *testing.T) releaseStageFixture {
	t.Helper()
	repository := t.TempDir()
	runReleaseGit(t, repository, "init")
	runReleaseGit(t, repository, "config", "user.email", "release-test@example.invalid")
	runReleaseGit(t, repository, "config", "user.name", "Release Test")
	writeReleaseTestFile(t, filepath.Join(repository, "tracked.txt"), "tracked\n")
	runReleaseGit(t, repository, "add", "tracked.txt")
	runReleaseGit(t, repository, "commit", "-m", "fixture")
	commit := strings.TrimSpace(runReleaseGit(t, repository, "rev-parse", "HEAD"))
	dist := filepath.Join(repository, "dist")
	stage := filepath.Join(dist, ".release-stage")
	source, output := filepath.Join(stage, "source"), filepath.Join(stage, "output")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReleaseTestFile(t, filepath.Join(source, "source.txt"), "original\n")
	tarPath := filepath.Join(stage, "source.tar")
	command := exec.Command("tar", "-cf", tarPath, "-C", source, ".")
	if raw, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create source archive: %v: %s", err, raw)
	}
	manifest := filepath.Join(stage, "source-manifest")
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	command = exec.Command("bash", "-c", `source "$1"; write_source_manifest "$2" "$3"`,
		"release-source-manifest", script, source, manifest)
	if raw, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create source manifest: %v: %s", err, raw)
	}
	writeReleaseTestFile(t, filepath.Join(output, "artifact"), "sealed\n")
	return releaseStageFixture{repository, dist, commit, stage, source, output, manifest}
}

func runReleasePublish(t *testing.T, fixture releaseStageFixture, before string) (string, error) {
	t.Helper()
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	source := `
source "$1"
REPO_ROOT="$2"; DIST_DIR="$3"; RELEASE_COMMIT="$4"
SOURCE_STAGE_ROOT="$5"; SOURCE_TREE="$6"; RELEASE_OUTPUT_STAGE="$7"
EXPECTED_SOURCE_MANIFEST="$8"; VERSION="v1.2.3"
RELEASE_SOURCE_SHA256="$(sha256_file "${SOURCE_STAGE_ROOT}/source.tar")"
` + before + "\npublish_staged_release\n"
	command := exec.Command("bash", "-c", source, "release-publish", script,
		fixture.repository, fixture.dist, fixture.commit, fixture.stage,
		fixture.source, fixture.output, fixture.manifest)
	raw, err := command.CombinedOutput()
	return string(raw), err
}

func assertNoReleasePublication(t *testing.T, dist string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dist, "omnidex-v1.2.3")); !os.IsNotExist(err) {
		t.Fatalf("failed publication left a visible release: %v", err)
	}
}

func runReleaseGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return string(output)
}
