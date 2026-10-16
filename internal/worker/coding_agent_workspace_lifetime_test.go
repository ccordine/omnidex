package worker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/workspace"
)

func TestNativeTaskLifetimeAndHostFenceSurviveCLIDisconnect(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	fence, err := workspace.AcquireMutationFence(fixture.ctx, fixture.scope.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := fence.Release(); err != nil {
			t.Error(err)
		}
	}()
	agentContext, cancel, err := fixture.service.nativeAgentWorkspaceContext(fixture.ctx, fixture.job, fixture.scope, fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if err := fixture.client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := agentContext.Err(); err != nil {
		t.Fatalf("CLI disconnect cancelled native task: %v", err)
	}
	if err := fence.Reattest(fixture.scope.Root); err != nil {
		t.Fatalf("CLI disconnect revoked the host-owned fence: %v", err)
	}
	cancel()
	if !errors.Is(agentContext.Err(), context.Canceled) {
		t.Fatal("explicit native cancellation no longer works")
	}
}

func TestNativeTaskCancellationDoesNotRevokeCLIConnection(t *testing.T) {
	fixture := newLocalAgentWorkspaceFixture(t, strings.Repeat("1", 32))
	agentContext, cancel, err := fixture.service.nativeAgentWorkspaceContext(fixture.ctx, fixture.job, fixture.scope, fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if !errors.Is(agentContext.Err(), context.Canceled) {
		t.Fatal("explicit native cancellation no longer works")
	}
	if err := fixture.service.workspaceConnections.Require(fixture.ctx, fixture.scope.Root, fixture.scope.Identity); err != nil {
		t.Fatalf("cancelling native work revoked the CLI connection: %v", err)
	}
}
