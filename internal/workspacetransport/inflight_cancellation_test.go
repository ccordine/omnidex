package workspacetransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspace"
)

type blockedPreparation struct{ entered chan struct{} }

func (p *blockedPreparation) ApplyVerified(ctx context.Context, _ workspace.VerifiedChangeObserver) (workspace.ReconciliationResult, error) {
	close(p.entered)
	<-ctx.Done()
	return workspace.ReconciliationResult{}, ctx.Err()
}

func TestDisconnectCancelsInflightClientOperationAndReleasesItsFence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root := t.TempDir()
	directory := retainedTestDirectory(t, root)
	directoryID, err := directory.Identity()
	if err != nil {
		t.Fatal(err)
	}
	clientID := strings.Repeat("4", 32)
	identity, err := projectroot.ClientWorkspaceIdentity(clientID, directoryID)
	if err != nil {
		t.Fatal(err)
	}
	remotes := make(chan *remote, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		r := newRemote(conn, binding{Root: root, Identity: identity})
		r.nextID = 3
		remotes <- r
		<-r.done
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetReadLimit(MaxFrameBytes)
	fence, err := workspace.AcquireMutationFence(ctx, root)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	localCtx, localCancel := context.WithCancel(ctx)
	blocked := &blockedPreparation{entered: make(chan struct{})}
	// The prepared operation is deliberately blocked to isolate transport
	// cancellation. The directory and native mutation fence are real.
	local := &Local{
		conn: conn, handle: directory, clientID: clientID, identity: identity,
		ctx: localCtx, cancel: localCancel, done: make(chan struct{}), nextID: 3,
		fence: fence, lease: 2, prepared: blocked, preparedID: 3,
	}
	go local.serve()
	defer local.Close()
	r := <-remotes
	defer r.stop(errors.New("test ended"))
	callCtx, cancelCall := context.WithCancel(ctx)
	defer cancelCall()
	result := make(chan error, 1)
	go func() {
		_, err := r.call(callCtx, requestData{request: request{Kind: opApply, Lease: 2, Prepared: 3}}, nil)
		result <- err
	}()
	select {
	case <-blocked.entered:
	case <-ctx.Done():
		t.Fatal("client operation did not begin")
	}
	cancelCall()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled server operation: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("server operation did not stop")
	}
	select {
	case <-local.Done():
	case <-ctx.Done():
		t.Fatal("client operation did not observe the disconnect")
	}
	reacquired, err := workspace.TryAcquireMutationFence(ctx, root)
	if err != nil {
		t.Fatalf("canceled operation retained native fence: %v", err)
	}
	if err := reacquired.Release(); err != nil {
		t.Fatal(err)
	}
}
