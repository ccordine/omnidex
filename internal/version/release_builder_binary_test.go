package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseBuilderRejectsTrackedExecutableByContentOutsideBuildPaths(t *testing.T) {
	repository := t.TempDir()
	runReleaseGit(t, repository, "init")
	path := filepath.Join(repository, "omnidex")
	if err := os.WriteFile(path, []byte("\x7fELF\x02\x01\x01\x00fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	runReleaseGit(t, repository, "add", "--", "omnidex")
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	command := exec.Command(
		"bash", "-c", `source "$1"; validate_tracked_release_sources "$2"`,
		"release-tracked-source-test", script, repository,
	)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "tracked generated artifact") ||
		!strings.Contains(string(output), "omnidex") {
		t.Fatalf("tracked executable error=%v output=%q", err, output)
	}
}
