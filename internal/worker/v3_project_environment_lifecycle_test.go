package worker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDirectCodingDockerEnvironmentCloseLeavesImmutableImageCache(t *testing.T) {
	root := t.TempDir()
	recorder := &recordingDirectCodingDockerInvoker{}
	environment, err := newDirectCodingDockerProjectEnvironment(
		testDirectCodingDockerEnvironmentSpec(),
		directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
		1234, 5678, recorder.invoke,
	)
	if err != nil {
		t.Fatalf("new environment: %v", err)
	}
	if err := environment.Build(context.Background()); err != nil {
		t.Fatalf("build environment: %v", err)
	}
	if err := environment.Close(context.Background()); err != nil {
		t.Fatalf("close environment: %v", err)
	}
	if err := environment.Close(context.Background()); err != nil {
		t.Fatalf("idempotent close: %v", err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("close issued destructive Docker cleanup: %#v", recorder.calls)
	}
	if _, err := environment.Run(context.Background(), directCodingProjectEnvironmentCommand{
		Program: "test", Args: []string{"-e", "after-close"}, Timeout: time.Second,
	}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("post-close run error=%v", err)
	}
}

func TestDirectCodingDockerEnvironmentCloseCannotInvalidateConcurrentHandle(t *testing.T) {
	root := t.TempDir()
	recorder := &recordingDirectCodingDockerInvoker{}
	first, err := newDirectCodingDockerProjectEnvironment(
		testDirectCodingDockerEnvironmentSpec(),
		directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
		1234, 5678, recorder.invoke,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newDirectCodingDockerProjectEnvironment(
		testDirectCodingDockerEnvironmentSpec(),
		directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
		1234, 5678, recorder.invoke,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Build(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := second.Build(context.Background()); err != nil {
		t.Fatal(err)
	}
	if first.ImageID() != second.ImageID() {
		t.Fatal("identical build inputs did not resolve to one immutable image ID")
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Run(context.Background(), directCodingProjectEnvironmentCommand{
		Program: "test", Args: []string{"-e", "second-handle"}, Timeout: time.Second,
	}); err != nil {
		t.Fatalf("first handle invalidated shared immutable image: %v", err)
	}
	for _, call := range recorder.calls {
		joined := strings.Join(call.Args, " ")
		if strings.Contains(joined, "image rm") || strings.Contains(joined, "prune") || strings.Contains(joined, "--force") {
			t.Fatalf("handle close issued destructive cleanup: %s", joined)
		}
	}
}
