package experiment

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerExperimentsExecuteAndCollectWithoutHostMounts(t *testing.T) {
	image := dockerFixtureImage(t)
	secret := "private host environment"
	t.Setenv("OMNIDEX_EXPERIMENT_TEST_SECRET", secret)
	hostFile := filepath.Join(t.TempDir(), "host.txt")
	if err := os.WriteFile(hostFile, []byte("host state"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name                          string
		argv                          []string
		input                         []File
		collect                       []string
		stdin, stdout, stderr, output []byte
	}{
		{"binary", []string{"/bin/sh", "-c", `test -z "${OMNIDEX_EXPERIMENT_TEST_SECRET+x}" && test "$(id -u)" = 65532 && test ! -e "$1" && test ! -e /var/run/docker.sock && cat nested/input.bin > result.bin && printf 'diagnostic\n' >&2 && cat`, "fixture", hostFile},
			[]File{{Path: "nested/input.bin", Content: []byte{0, 1, '\r', '\n', 255}, Mode: 0o640}}, []string{"result.bin", "nested/input.bin"}, []byte("stdin\x00exact"), []byte("stdin\x00exact"), []byte("diagnostic\n"), []byte{0, 1, '\r', '\n', 255}},
		{"arithmetic", []string{"/bin/sh", "-c", `awk '{ total += $1 } END { print total }' values.txt > total.txt; cat total.txt`},
			[]File{{Path: "values.txt", Content: []byte("3\n9\n-2\n"), Mode: 0o644}}, []string{"total.txt"}, nil, []byte("10\n"), nil, []byte("10\n")},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			result, err := NewDocker().Run(context.Background(), Request{ImageID: image, Argv: fixture.argv, Input: fixture.input, Collect: fixture.collect, Stdin: fixture.stdin, Timeout: 30 * time.Second})
			if err != nil {
				t.Fatalf("%v; stdout=%q stderr=%q", err, result.Stdout, result.Stderr)
			}
			if result.ImageID != image || result.ExitCode == nil || *result.ExitCode != 0 || result.StartedAt.IsZero() || result.FinishedAt.Before(result.StartedAt) {
				t.Fatalf("missing actual process observation: %+v", result)
			}
			if !bytes.Equal(result.Stdout, fixture.stdout) || !bytes.Equal(result.Stderr, fixture.stderr) || len(result.Files) != len(fixture.collect) || !bytes.Equal(result.Files[0].Content, fixture.output) {
				t.Fatalf("experiment bytes changed: stdout=%q stderr=%q files=%+v", result.Stdout, result.Stderr, result.Files)
			}
			if fixture.name == "binary" && (result.Files[1].Mode != 0o640 || !bytes.Equal(result.Files[1].Content, fixture.input[0].Content)) {
				t.Fatal("input bytes or permissions changed")
			}
			assertDockerFixtureRemoved(t, result.ContainerID)
		})
	}
	content, err := os.ReadFile(hostFile)
	if err != nil || string(content) != "host state" {
		t.Fatalf("Docker changed host state: %q %v", content, err)
	}
}

func TestDockerExperimentFailuresAreObservedAndCleanedUp(t *testing.T) {
	image := dockerFixtureImage(t)
	for _, fixture := range []struct {
		name, script string
		collect      []string
		timeout      time.Duration
	}{
		{"exit", "printf observed; printf failure >&2; exit 7", []string{"missing"}, 30 * time.Second},
		{"timeout", "trap '' TERM; sleep 60 & wait", nil, 3 * time.Second},
		{"overflow", "yes data", nil, 30 * time.Second},
		{"link", "ln -s /etc/passwd result", []string{"result"}, 30 * time.Second},
		{"linked_parent", "ln -s /etc result", []string{"result/passwd"}, 30 * time.Second},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			result, err := NewDocker().Run(context.Background(), Request{ImageID: image, Argv: []string{"/bin/sh", "-c", fixture.script}, Collect: fixture.collect, Timeout: fixture.timeout})
			if err == nil || len(result.Files) != 0 {
				t.Fatalf("failed experiment returned artifacts: %+v %v", result, err)
			}
			switch fixture.name {
			case "exit":
				if result.ExitCode == nil || *result.ExitCode != 7 || string(result.Stdout) != "observed" || string(result.Stderr) != "failure" {
					t.Fatalf("lost actual failed process result: %+v %v", result, err)
				}
			case "timeout":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("timeout was obscured: %v", err)
				}
			case "overflow":
				if len(result.Stdout) != MaxStreamBytes || !strings.Contains(err.Error(), "stream bound") {
					t.Fatalf("output bound was not enforced: bytes=%d %v", len(result.Stdout), err)
				}
			case "link", "linked_parent":
				if !strings.Contains(err.Error(), "not a bounded regular file") && !strings.Contains(err.Error(), "not an exact directory") {
					t.Fatalf("link failed for another reason: %v", err)
				}
			}
			assertDockerFixtureRemoved(t, result.ContainerID)
		})
	}
}

func dockerFixtureImage(t *testing.T) string {
	t.Helper()
	image := os.Getenv("OMNI_TEST_EXPERIMENT_IMAGE_ID")
	if image == "" {
		t.Skip("OMNI_TEST_EXPERIMENT_IMAGE_ID is required for real Docker execution")
	}
	if !imageIDPattern.MatchString(image) {
		t.Fatal("Docker fixture requires an observed immutable image ID")
	}
	return image
}

func assertDockerFixtureRemoved(t *testing.T, id string) {
	t.Helper()
	if !containerIDPattern.MatchString(id) {
		t.Fatal("experiment has no observed container identity")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "container", "ls", "--all", "--quiet", "--no-trunc", "--filter", "id="+id).CombinedOutput()
	if err != nil || len(bytes.TrimSpace(output)) != 0 {
		t.Fatalf("owned experiment remains: %q %v", output, err)
	}
}
