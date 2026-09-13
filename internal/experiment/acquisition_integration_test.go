package experiment

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAcquisitionSealsNetworkBeforeOrdinaryExecution(t *testing.T) {
	image := dockerFixtureImage(t)
	for _, fixture := range []struct{ name, input, prepare, finish, want string }{
		{"text", "Mixed Case", "tr a-z A-Z < input > retained", "cat retained", "MIXED CASE"},
		{"numbers", "4\n6\n", "awk '{total += $1} END {print total}' input > retained", "awk '{print $1 * 3}' retained", "30\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			workspace, err := NewDocker().OpenAcquisition(ctx, image, []File{{Path: "input", Content: []byte(fixture.input), Mode: 0o600}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := workspace.Close(); err != nil {
					t.Error(err)
				}
				assertDockerFixtureRemoved(t, workspace.containerID)
			})
			if result, err := workspace.Run(ctx, Command{Argv: []string{"/bin/true"}, Timeout: time.Second}); err == nil || result.ExecutionID != "" {
				t.Fatalf("ordinary execution borrowed acquisition network: %+v %v", result, err)
			}
			if err := workspace.Write(ctx, []File{{Path: "unaccepted", Content: []byte("content"), Mode: 0o600}}); err == nil {
				t.Fatal("source transfer used acquisition network")
			}
			acquired, err := workspace.Acquire(ctx, Command{Argv: []string{"/bin/sh", "-c", "test -e /sys/class/net/eth0 && " + fixture.prepare}, Timeout: 10 * time.Second})
			if err != nil || acquired.NetworkEnabled == nil || !*acquired.NetworkEnabled {
				t.Fatalf("acquisition = %+v %v", acquired, err)
			}
			if err := workspace.SealNetwork(ctx); err != nil {
				t.Fatal(err)
			}
			if err := workspace.SealNetwork(ctx); err != nil {
				t.Fatalf("already sealed workspace changed: %v", err)
			}
			result, err := workspace.Run(ctx, Command{Argv: []string{"/bin/sh", "-c", "test ! -e /sys/class/net/eth0 && " + fixture.finish}, Timeout: 10 * time.Second})
			if err != nil || string(result.Stdout) != fixture.want || result.NetworkEnabled == nil || *result.NetworkEnabled {
				t.Fatalf("sealed execution = %+v %v", result, err)
			}
			if acquired.ContainerID != result.ContainerID {
				t.Fatal("network sealing lost acquired files")
			}
			if _, err := workspace.Acquire(ctx, Command{Argv: []string{"/bin/true"}, Timeout: time.Second}); err == nil || !strings.Contains(err.Error(), "acquisition") {
				t.Fatalf("sealed acquisition was reopened: %v", err)
			}
		})
	}
}
