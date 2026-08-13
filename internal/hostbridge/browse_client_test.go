package hostbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientBrowseBindsRequestedPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("path") != "/workspace" || r.URL.Query().Get("limit") != "7" ||
			r.URL.Query().Get("offset") != "14" || r.URL.Query().Get("directories_only") != "true" {
			t.Fatalf("unexpected browse query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": "/workspace", "entries": []any{},
			"limit": 7, "offset": 14, "has_previous": true, "previous_offset": 7,
			"has_more": true, "next_offset": 21,
		})
	}))
	t.Cleanup(server.Close)

	result, err := NewClient(server.URL, "", 0).Browse(context.Background(), "/workspace", BrowseOptions{
		Limit: 7, Offset: 14, DirectoriesOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Limit != 7 || result.Offset != 14 || !result.HasMore || result.NextOffset != 21 {
		t.Fatalf("result=%+v", result)
	}
}

func TestClientBrowseRejectsUnpagedHostResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"path": "/workspace", "entries": []any{}})
	}))
	t.Cleanup(server.Close)

	if _, err := NewClient(server.URL, "", 0).Browse(context.Background(), "/workspace", BrowseOptions{Limit: 1}); err == nil {
		t.Fatal("expected missing page authority to fail")
	}
}
