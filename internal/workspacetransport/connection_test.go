package workspacetransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWorkspaceConnectionAttestsInvokingDirectoryAndRejectsReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hub := NewHub()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = hub.Serve(r.Context(), conn)
		}
	}))
	defer server.Close()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	directory := retainedTestDirectory(t, root)
	local, err := Connect(ctx, conn, directory, strings.Repeat("1", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	if err := hub.Require(ctx, root, local.Identity()); err != nil {
		t.Fatalf("attest current client directory: %v", err)
	}
	if err := hub.Require(ctx, root+"-other", local.Identity()); err == nil {
		t.Fatal("accepted different workspace path")
	}
	if err := os.Rename(root, root+"-retired"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := hub.Require(ctx, root, local.Identity()); err == nil {
		t.Fatal("replaced client directory retained authority")
	}
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	if err := hub.Require(ctx, root, local.Identity()); err == nil {
		t.Fatal("disconnected client retained authority")
	}
}

func TestWorkspaceHubNeverUsesMatchingServerDirectoryWithoutConnection(t *testing.T) {
	if err := NewHub().Require(context.Background(), t.TempDir(),
		"client_"+strings.Repeat("1", 32)+"_directory_1_2"); err == nil {
		t.Fatal("server directory substituted for missing client connection")
	}
}

func TestConcurrentWorkspaceAttestationsAndDuplicateConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hub := NewHub()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = hub.Serve(r.Context(), conn)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	connect := func(clientID string) (*Local, error) {
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			return nil, err
		}
		return Connect(ctx, conn, retainedTestDirectory(t, root), clientID)
	}
	first, err := connect(strings.Repeat("1", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := connect(strings.Repeat("2", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if duplicate, err := connect(strings.Repeat("1", 32)); err == nil {
		_ = duplicate.Close()
		t.Fatal("duplicate connection replaced a live client")
	}
	var group sync.WaitGroup
	for range 20 {
		for _, local := range []*Local{first, second} {
			group.Add(1)
			go func() {
				defer group.Done()
				if err := hub.Require(ctx, root, local.Identity()); err != nil {
					t.Errorf("concurrent attestation: %v", err)
				}
			}()
		}
	}
	group.Wait()
}
