package workspacetransport

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspace"
)

func connectedWorkspace(t *testing.T) (context.Context, *Hub, *Local, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	hub := NewHub()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = hub.Serve(r.Context(), conn)
		}
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	local, err := Connect(ctx, conn, retainedTestDirectory(t, root), strings.Repeat("1", 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	return ctx, hub, local, root
}

func TestClientWorkspaceReadsAndPublishesExactFiles(t *testing.T) {
	for _, content := range [][]byte{[]byte("first\r\nsecond\n"), bytes.Repeat([]byte{0, 1, 255}, 400000)} {
		t.Run(string(rune('a'+len(content)%26)), func(t *testing.T) {
			ctx, hub, local, root := connectedWorkspace(t)
			if err := os.WriteFile(filepath.Join(root, "original"), content, 0o640); err != nil {
				t.Fatal(err)
			}
			access, err := hub.Acquire(ctx, root, local.Identity())
			if err != nil {
				t.Fatal(err)
			}
			defer access.Release()
			original, err := access.ReadFile(ctx, "original")
			if err != nil || !bytes.Equal(original.Content, content) || original.Entry.Mode != 0o640 {
				t.Fatalf("read bytes=%d mode=%o err=%v", len(original.Content), original.Entry.Mode, err)
			}
			if _, err := access.Stat(ctx, "missing"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing: %v", err)
			}
			page, err := access.ReadDirectory(ctx, ".", "", 1)
			if err != nil || len(page.Entries) != 1 || page.Entries[0].Path != "original" || page.HasMore {
				t.Fatalf("page=%#v err=%v", page, err)
			}
			desired := []workspace.DesiredFile{{Path: "nested/result", Present: true, Content: content, Mode: 0o600, CreateOnly: true}}
			prepared, err := access.Prepare(ctx, desired, []workspace.File{original})
			if err != nil {
				t.Fatal(err)
			}
			var observed []workspace.Change
			result, err := prepared.ApplyVerified(ctx, func(change workspace.Change) { observed = append(observed, change) })
			if err != nil || len(result.Changes) != 1 || len(observed) != 1 || observed[0] != result.Changes[0] {
				t.Fatalf("result=%#v observed=%#v err=%v", result, observed, err)
			}
			actual, err := os.ReadFile(filepath.Join(root, "nested/result"))
			if err != nil || !bytes.Equal(actual, content) {
				t.Fatalf("published bytes=%d err=%v", len(actual), err)
			}
			if _, err := prepared.ApplyVerified(ctx, nil); err == nil {
				t.Fatal("reused applied preparation")
			}
			if err := hub.Require(ctx, root, local.Identity()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientWorkspacePreservesEditsAndReportsPartialPublication(t *testing.T) {
	ctx, hub, local, root := connectedWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "existing"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := hub.Acquire(ctx, root, local.Identity())
	if err != nil {
		t.Fatal(err)
	}
	defer access.Release()
	expected, err := access.ReadFile(ctx, "existing")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := access.Prepare(ctx, []workspace.DesiredFile{{Path: "new", Present: true, Content: []byte("value"), Mode: 0o600}}, []workspace.File{expected})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing"), []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := prepared.ApplyVerified(ctx, nil)
	if err == nil || len(result.Changes) != 0 {
		t.Fatalf("edited input: %#v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "z"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err = access.Prepare(ctx, []workspace.DesiredFile{
		{Path: "a", Present: true, Content: []byte("accepted"), Mode: 0o600},
		{Path: "z/child", Present: true, Content: []byte("unreachable"), Mode: 0o600, CreateOnly: true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var observed []workspace.Change
	result, err = prepared.ApplyVerified(ctx, func(change workspace.Change) { observed = append(observed, change) })
	if err == nil || len(result.Changes) != 1 || len(observed) != 1 || observed[0].Path != "a" {
		t.Fatalf("partial result=%#v observed=%#v err=%v", result, observed, err)
	}
	if err := hub.Require(ctx, root, local.Identity()); err != nil {
		t.Fatal(err)
	}
}

func retainedTestDirectory(t *testing.T, root string) *projectroot.DirectoryHandle {
	t.Helper()
	directory, err := projectroot.OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := directory.Close(); err != nil {
			t.Error(err)
		}
	})
	return directory
}
