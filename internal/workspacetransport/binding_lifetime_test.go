package workspacetransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWorkspaceBindingLifetimeCancellationPreservesOtherOwners(t *testing.T) {
	ctx, hub, local, root := connectedWorkspace(t)
	parent, cancelParent := context.WithCancel(ctx)
	lifetime, cancel, err := hub.Lifetime(parent, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	cancelParent()
	if !errors.Is(lifetime.Err(), context.Canceled) {
		t.Fatalf("native lifetime ignored caller cancellation: %v", lifetime.Err())
	}
	if err := hub.Require(ctx, root, local.Identity()); err != nil {
		t.Fatalf("caller cancellation closed the CLI connection: %v", err)
	}
	independent, release, err := hub.Lifetime(ctx, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	release()
	if !errors.Is(independent.Err(), context.Canceled) {
		t.Fatal("releasing native lifetime did not cancel its work")
	}
	if err := hub.Require(ctx, root, local.Identity()); err != nil {
		t.Fatalf("releasing a native lifetime closed the CLI connection: %v", err)
	}
}

func TestWorkspaceBindingLifetimeCannotMoveToReconnectedClient(t *testing.T) {
	ctx, hub, local, root := connectedWorkspace(t)
	lifetime, cancel, err := hub.Lifetime(ctx, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	oldAccess, err := hub.Acquire(ctx, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lifetime.Done():
	case <-time.After(time.Second):
		t.Fatal("native lifetime survived its exact connection")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		connection, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err == nil {
			_ = hub.Serve(request.Context(), connection)
		}
	}))
	defer server.Close()
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	reconnected, err := Connect(ctx, connection, retainedTestDirectory(t, root), strings.Repeat("1", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	if err := hub.Require(ctx, root, reconnected.Identity()); err != nil {
		t.Fatalf("fresh client connection failed: %v", err)
	}
	if !errors.Is(lifetime.Err(), context.Canceled) {
		t.Fatal("reconnection restored an earlier native grant")
	}
	if err := oldAccess.Reattest(root); err == nil {
		t.Fatal("old writer acquired the reconnected client's mutation authority")
	}
	if err := oldAccess.Release(); err == nil {
		t.Fatal("old lease survived its connection")
	}
	fresh, releaseFresh, err := hub.Lifetime(ctx, root, reconnected.Identity())
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFresh()
	if err := fresh.Err(); err != nil {
		t.Fatalf("new connection inherited old revocation: %v", err)
	}
}

func TestWorkspaceBindingLifetimeRequiresExactLiveDirectory(t *testing.T) {
	ctx, hub, local, root := connectedWorkspace(t)
	for _, binding := range []struct{ root, identity string }{
		{root + "-other", local.Identity()},
		{root, "client_" + strings.Repeat("2", 32) + "_directory_1_2"},
	} {
		if _, cancel, err := hub.Lifetime(ctx, binding.root, binding.identity); err == nil {
			cancel()
			t.Fatalf("native work acquired an unbound directory: %#v", binding)
		}
	}
	if err := os.Rename(root, root+"-retired"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, cancel, err := hub.Lifetime(ctx, root, local.Identity()); err == nil {
		cancel()
		t.Fatal("native lifetime attested a replaced directory")
	}
}
