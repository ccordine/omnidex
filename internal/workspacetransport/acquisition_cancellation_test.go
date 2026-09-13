package workspacetransport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/projectroot"
)

func TestCanceledAcquisitionReleasesGrantReceivedAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root := t.TempDir()
	directoryID, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := projectroot.ClientWorkspaceIdentity(strings.Repeat("5", 32), directoryID)
	if err != nil {
		t.Fatal(err)
	}
	remotes := make(chan *remote, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		r := newRemote(conn, binding{Root: root, Identity: identity})
		close(r.ready)
		remotes <- r
		<-r.done
	}))
	defer server.Close()
	peer, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if err := peer.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	r := <-remotes
	defer r.stop(errors.New("test ended"))
	hub := NewHub()
	hub.connections[identity] = r
	callCtx, cancelCall := context.WithCancel(ctx)
	defer cancelCall()
	result := make(chan error, 1)
	go func() {
		access, err := hub.Acquire(callCtx, root, identity)
		if access != nil {
			err = errors.Join(fmt.Errorf("canceled acquisition returned a lease"), access.Release())
		}
		result <- err
	}()
	var acquire request
	if err := readExact(peer, &acquire); err != nil {
		t.Fatal(err)
	}
	if acquire.Kind != opAcquire {
		t.Fatalf("request=%#v", acquire)
	}
	// The client has acquired authority, but its response is still in flight.
	// Cancellation cannot make that grant disappear or discard its identity.
	cancelCall()
	if err := writeExact(peer, response{ID: acquire.ID, Identity: identity}); err != nil {
		t.Fatal(err)
	}
	var release request
	if err := readExact(peer, &release); err != nil {
		t.Fatalf("late grant was not released: %v", err)
	}
	if release.Kind != opRelease || release.Lease != acquire.ID {
		t.Fatalf("released a different grant: %#v", release)
	}
	if err := writeExact(peer, response{ID: release.ID, Identity: identity}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled acquisition: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("canceled acquisition did not complete its cleanup")
	}
}
