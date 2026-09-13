package experiment

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestDockerWorkspaceRetainsDependenciesAcrossCommands(t *testing.T) {
	image := dockerFixtureImage(t)
	for _, fixture := range []struct {
		name          string
		input, want   []byte
		first, second string
	}{
		{"binary", []byte{'a', 0, 'b', 255}, []byte{'A', 0, 'B', 255}, "cp input intermediate", "tr a-z A-Z < intermediate > result"},
		{"arithmetic", []byte("3\n9\n-2\n"), []byte("20\n"), "awk '{total += $1} END {print total}' input > intermediate", "awk '{print $1 * 2}' intermediate > result"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			workspace, err := NewDocker().Open(context.Background(), image, []File{{Path: "input", Content: fixture.input, Mode: 0o640}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := workspace.Close(); err != nil {
					t.Error(err)
				}
				assertDockerFixtureRemoved(t, workspace.containerID)
			})
			first, err := workspace.Run(context.Background(), Command{Argv: []string{"/bin/sh", "-c", fixture.first}, Timeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			failed, err := workspace.Run(context.Background(), Command{Argv: []string{"/bin/sh", "-c", "exit 7"}, Timeout: 10 * time.Second})
			var exited *ExitError
			if !errors.As(err, &exited) || exited.Code != 7 || failed.ExitCode == nil || *failed.ExitCode != 7 {
				t.Fatalf("failed process result = %+v, %v", failed, err)
			}
			second, err := workspace.Run(context.Background(), Command{Argv: []string{"/bin/sh", "-c", fixture.second}, Timeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			if first.ContainerID != second.ContainerID || first.ExecutionID == second.ExecutionID || !containerIDPattern.MatchString(second.ExecutionID) {
				t.Fatal("commands lost their shared workspace or distinct process identity")
			}
			files, err := workspace.Collect(context.Background(), []string{"input", "result"})
			if err != nil || len(files) != 2 || !bytes.Equal(files[0].Content, fixture.input) || !bytes.Equal(files[1].Content, fixture.want) {
				t.Fatalf("retained artifacts = %+v, %v", files, err)
			}
		})
	}
}

func TestDockerWorkspaceTimeoutPreventsLaterDispatch(t *testing.T) {
	workspace, err := NewDocker().Open(context.Background(), dockerFixtureImage(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := workspace.Close(); err != nil {
			t.Error(err)
		}
		assertDockerFixtureRemoved(t, workspace.containerID)
	})
	_, err = workspace.Run(context.Background(), Command{Argv: []string{"/bin/sh", "-c", "sleep 60"}, Timeout: time.Second})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout = %v", err)
	}
	if result, err := workspace.Run(context.Background(), Command{Argv: []string{"/bin/true"}, Timeout: time.Second}); err == nil || result.ExecutionID != "" {
		t.Fatalf("closed experiment dispatched another command: %+v %v", result, err)
	}
}
