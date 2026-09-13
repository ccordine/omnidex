package workspacetransport

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/workspace"
)

func TestBusyClientWorkspaceKeepsAttestationAvailableAndReleasesOnDisconnect(t *testing.T) {
	ctx, hub, local, root := connectedWorkspace(t)
	first, err := hub.Acquire(ctx, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	waiting := make(chan error, 1)
	go func() {
		access, err := hub.Acquire(waitCtx, root, local.Identity())
		if access != nil {
			_ = access.Release()
		}
		waiting <- err
	}()
	if err := hub.Require(ctx, root, local.Identity()); err != nil {
		t.Fatal(err)
	}
	if err := <-waiting; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("busy acquisition: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := hub.Acquire(ctx, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Stat(ctx, "."); err == nil {
		t.Fatal("released access retained authority")
	}
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Stat(ctx, "."); err == nil {
		t.Fatal("disconnected access retained authority")
	}
	native, err := workspace.TryAcquireMutationFence(ctx, root)
	if err != nil {
		t.Fatalf("disconnection retained native mutation lock: %v", err)
	}
	if err := native.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestClientPreparationSupportsMovesAndParentReplacement(t *testing.T) {
	ctx, hub, local, root := connectedWorkspace(t)
	access, err := hub.Acquire(ctx, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	defer access.Release()
	initial, err := access.Prepare(ctx, []workspace.DesiredFile{
		{Path: "parent", Present: true, Content: []byte("occupied"), Mode: 0o600},
		{Path: "source", Present: true, Content: []byte("old"), Mode: 0o600},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := initial.ApplyVerified(ctx, nil); err != nil {
		t.Fatal(err)
	}
	prepared, err := access.Prepare(ctx, []workspace.DesiredFile{
		{Path: "parent/child", Present: true, Content: []byte("nested"), Mode: 0o600},
		{Path: "destination", Present: true, Content: []byte("changed"), Mode: 0o600, MoveFrom: "source"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.ApplyVerified(ctx, nil)
	if err != nil || len(result.Changes) != 4 {
		t.Fatalf("move and parent replacement: %#v %v", result, err)
	}
}
