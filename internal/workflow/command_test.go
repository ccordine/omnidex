package workflow

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolOutputTrackingAndOriginalExitStatus(t *testing.T) {
	scope := fixtureScope(t)
	scope.Go = filepath.Join(t.TempDir(), "go")
	script := "#!/bin/sh\nif [ \"$1\" = fail ]; then printf 'original-error\\n' >&2; exit 23; fi\nwhile [ $# -gt 0 ]; do if [ \"$1\" = -o ]; then shift; printf binary > \"$1\"; fi; shift; done\nprintf 'original-output\\n'\n"
	if err := os.WriteFile(scope.Go, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	file := filepath.Join(scope.Workspace, "binary")
	if err := Execute(t.Context(), scope, "go", []string{"build", "-o", file, "./cmd/example"}, nil, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if out.String() != "original-output\n" {
		t.Fatal("changed original output", out.String())
	}
	records, err := scope.records()
	if err != nil || len(records) != 1 || records[0].Path != file {
		t.Fatal("output not tracked", records, err)
	}
	err = Execute(t.Context(), scope, "go", []string{"fail"}, nil, &out, &stderr)
	if failure, ok := err.(*exec.ExitError); !ok || failure.ExitCode() != 23 || stderr.String() != "original-error\n" {
		t.Fatal("original failure was changed", err, stderr.String())
	}
}

func TestDockerOwnershipAndExactImageRetirement(t *testing.T) {
	scope := fixtureScope(t)
	log := filepath.Join(t.TempDir(), "commands")
	t.Setenv("WORKFLOW_TEST_LOG", log)
	t.Setenv("WORKFLOW_TEST_OWNER", key(scope.Workspace))
	scope.Docker = filepath.Join(t.TempDir(), "docker")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$WORKFLOW_TEST_LOG"
case "$1 $2" in
 'build '*) exit 0 ;;
 'image inspect') printf '[{"ID":"sha256:%s","Config":{"Labels":{"io.omnidex.workflow":"%s"}}}]\n' "$3" "$WORKFLOW_TEST_OWNER" ;;
 'container ls') exit 0 ;;
 'image rm') exit 0 ;;
 *) exit 1 ;;
esac
`
	if err := os.WriteFile(scope.Docker, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		scope.Task = fmt.Sprint(i)
		if err := Execute(t.Context(), scope, "docker", []string{"build", "-t", fmt.Sprint(i), "."}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := scope.Complete([]string{"0", "1", "2"}); err != nil {
		t.Fatal(err)
	}
	if err := scope.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	commands := string(data)
	if strings.Count(commands, "--label io.omnidex.workflow=") != 3 {
		t.Fatal("created objects lack ownership labels", commands)
	}
	if strings.Contains(commands, "io.omnidex.task=") {
		t.Fatal("per-item build labels defeat image reuse")
	}
	if !strings.Contains(commands, "image rm 0\n") || strings.Contains(commands, "image rm 1\n") || strings.Contains(commands, "image rm 2\n") || strings.Contains(commands, "prune") {
		t.Fatal("incorrect image retention/cleanup", commands)
	}
}

func TestBuildLineageReusesChangingReleaseFlagsAndSeparatesPlatforms(t *testing.T) {
	first := goFamily([]string{"build", "-trimpath", "-ldflags", "-X main.version=one", "-o", "one", "./cmd/app"})
	second := goFamily([]string{"build", "-trimpath", "-ldflags", "-X main.version=two", "-o", "two", "./cmd/app"})
	if first != second {
		t.Fatal("release names or flags allocate a new build family")
	}
	t.Setenv("GOOS", "different")
	if goFamily([]string{"build", "-o", "three", "./cmd/app"}) == first {
		t.Fatal("different target overwrites existing build lineage")
	}
}

func TestDockerBuildContextIgnoresFollowingFlags(t *testing.T) {
	for _, args := range [][]string{{"build", "first", "-t", "image:1"}, {"build", "--build-arg", "VERSION=two", "-f", "Dockerfile", "first", "--tag=image:2"}} {
		if dockerBuildContext(args) != "first" {
			t.Fatal("build context mixed with release metadata", args)
		}
	}
	if dockerBuildContext([]string{"build", "second", "-t", "image:1"}) == "first" {
		t.Fatal("distinct build inputs share a lineage")
	}
}

func TestExportUsesNewestReceiptFromTheSameEngine(t *testing.T) {
	scope := fixtureScope(t)
	if err := scope.Record(Resource{Kind: "image", Path: "fixture:1", ID: "sha256:old", Family: "old-family"}); err != nil {
		t.Fatal(err)
	}
	if err := scope.Record(Resource{Kind: "image", Path: "fixture:1", ID: "sha256:new", Family: "new-family"}); err != nil {
		t.Fatal(err)
	}
	if err := scope.Record(Resource{Kind: "image", Path: "fixture:1", ID: "sha256:other-engine", Family: "different-engine", Engine: []string{"--context", "other"}}); err != nil {
		t.Fatal(err)
	}
	scope.Docker = filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(scope.Docker, []byte("#!/bin/sh\nprintf archive > \"$3\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(scope.Workspace, "archive.tar")
	if err := Execute(t.Context(), scope, "docker", []string{"save", "-o", file, "fixture:1"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	records, err := scope.records()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if r.Kind == "file" {
			if r.ID != "sha256:new" || r.Family != "archive:new-family" {
				t.Fatal("old or different-engine receipt overrode the current export", r)
			}
			return
		}
	}
	t.Fatal("export was not tracked")
}

func TestFailedBuildOutputIsPreservedUntilAcceptanceThenRetired(t *testing.T) {
	scope := fixtureScope(t)
	scope.Go = filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(scope.Go, []byte("#!/bin/sh\nprintf partial > \"$3\"\nexit 2\n"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(scope.Workspace, "partial-output")
	if err := Execute(t.Context(), scope, "go", []string{"build", "-o", file, "./cmd/app"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("failed build reported success")
	}
	if err := scope.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("unfinished work was removed")
	}
	if err := scope.Complete([]string{scope.Task}); err != nil {
		t.Fatal(err)
	}
	if err := scope.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("partial output counted as usable rollback")
	}
}
