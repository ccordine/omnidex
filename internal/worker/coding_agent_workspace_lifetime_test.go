package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/workspace"
)

func TestLocalCLIAgentWorkspaceRevokesNativeWorkAndPreparedWritesOnDisconnect(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	access, err := fixture.service.workspaceConnections.Acquire(fixture.ctx, fixture.scope.Root, fixture.scope.Identity)
	if err != nil {
		t.Fatal(err)
	}
	agentContext, cancel, err := fixture.service.nativeAgentWorkspaceContext(fixture.ctx, fixture.job, fixture.scope, fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	prepared, err := access.Prepare(agentContext, []workspace.DesiredFile{{Path: "scoped.txt", Present: true, Content: []byte("authorized"), Mode: 0o600}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	inFlight := make(chan error, 1)
	go func() {
		<-agentContext.Done()
		inFlight <- agentContext.Err()
	}()
	if err := fixture.client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-inFlight:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("native work cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight native work retained authority after the CLI connection closed")
	}
	if !strings.Contains(context.Cause(agentContext).Error(), "connection closed") {
		t.Fatalf("native cancellation lost its exact connection failure: %v", context.Cause(agentContext))
	}
	if _, err := prepared.ApplyVerified(fixture.ctx, nil); err == nil {
		t.Fatal("prepared task write survived connection revocation")
	}
	if _, err := os.Stat(filepath.Join(fixture.scope.Root, "scoped.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revoked task write changed workspace: %v", err)
	}
	if err := access.Release(); err == nil {
		t.Fatal("revoked transport lease remained active")
	}
	fence, err := workspace.TryAcquireMutationFence(fixture.ctx, fixture.scope.Root)
	if err != nil {
		t.Fatalf("revocation retained the local mutation lock: %v", err)
	}
	if err := fence.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalCLIAgentWorkspaceKeepsExistingMutationFenceAndExactWriteBoundary(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	access, err := fixture.service.workspaceConnections.Acquire(fixture.ctx, fixture.scope.Root, fixture.scope.Identity)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := access.Release(); err != nil {
			t.Error(err)
		}
	}()
	agentContext, cancel, err := fixture.service.nativeAgentWorkspaceContext(fixture.ctx, fixture.job, fixture.scope, fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if fence, err := workspace.TryAcquireMutationFence(agentContext, fixture.scope.Root); !errors.Is(err, workspace.ErrWorkspaceBusy) {
		if fence != nil {
			_ = fence.Release()
		}
		t.Fatalf("native access replaced or duplicated CLI mutation authority: %v", err)
	}
	for _, path := range []string{"../outside.txt", filepath.Join(fixture.scope.Root, "absolute.txt")} {
		if _, err := access.Prepare(agentContext, []workspace.DesiredFile{{Path: path, Present: true, Mode: 0o600}}, nil); err == nil {
			t.Fatalf("task writer escaped exact root through %q", path)
		}
	}
	prepared, err := access.Prepare(agentContext, []workspace.DesiredFile{{Path: "scoped.txt", Present: true, Content: []byte("authorized"), Mode: 0o600}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.ApplyVerified(agentContext, nil); err != nil {
		t.Fatal(err)
	}
	if actual, err := os.ReadFile(filepath.Join(fixture.scope.Root, "scoped.txt")); err != nil || string(actual) != "authorized" {
		t.Fatalf("exact-root task writer failed: %q: %v", actual, err)
	}
	cancel()
	if _, err := access.ReadFile(agentContext, "scoped.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled native call retained read authority: %v", err)
	}
	if err := fixture.service.workspaceConnections.Require(fixture.ctx, fixture.scope.Root, fixture.scope.Identity); err != nil {
		t.Fatalf("canceling native work revoked another owner's CLI connection: %v", err)
	}
}
