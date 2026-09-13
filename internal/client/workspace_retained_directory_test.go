package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspacetransport"
)

func TestWorkspaceConnectionRejectsDirectoryReplacementDuringHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root := filepath.Join(t.TempDir(), "selected")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := projectroot.OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	hub := workspacetransport.NewHub()
	replacement := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			replacement <- err
			return
		}
		defer conn.Close()
		if err := os.Rename(root, root+"-retained"); err != nil {
			replacement <- err
			return
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			replacement <- err
			return
		}
		replacement <- nil
		_ = hub.Serve(r.Context(), conn)
	}))
	defer server.Close()
	client, err := New(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if connection, err := client.OpenWorkspace(ctx, directory, strings.Repeat("1", 32)); err == nil {
		_ = connection.Close()
		t.Fatal("network setup retargeted the replacement directory")
	}
	if err := <-replacement; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, root + "-retained"} {
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 0 {
			t.Fatalf("rejected setup changed %s: %v %v", path, entries, err)
		}
	}
}
