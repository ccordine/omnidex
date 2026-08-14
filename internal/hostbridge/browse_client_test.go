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
			r.URL.Query().Get("offset") != "14" || r.URL.Query().Get("directories_only") != "true" ||
			r.URL.Query().Get("required_root") != "/workspace" {
			t.Fatalf("unexpected browse query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": "/workspace", "entries": []any{},
			"limit": 7, "offset": 14, "has_previous": true, "previous_offset": 7,
			"has_more": true, "next_offset": 21, "required_root": "/workspace",
		})
	}))
	t.Cleanup(server.Close)

	result, err := NewClient(server.URL, "", 0).Browse(context.Background(), "/workspace", BrowseOptions{
		Limit: 7, Offset: 14, DirectoriesOnly: true, RequiredRoot: "/workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Limit != 7 || result.Offset != 14 || !result.HasMore || result.NextOffset != 21 {
		t.Fatalf("result=%+v", result)
	}
}

func TestClientBrowseRejectsLegacyBridgeWithoutRequiredRootAttestation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": "/workspace", "entries": []any{},
			"limit": 1, "offset": 0, "has_previous": false, "previous_offset": 0,
			"has_more": false, "next_offset": 0,
		})
	}))
	t.Cleanup(server.Close)
	if _, err := NewClient(server.URL, "", 0).Browse(context.Background(), "/workspace", BrowseOptions{
		Limit: 1, RequiredRoot: "/workspace",
	}); err == nil {
		t.Fatal("legacy bridge without required-root attestation was accepted")
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

func TestClientBrowseRejectsOversizedHostPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": "/workspace", "entries": []any{
				map[string]any{"name": "one", "path": "/workspace/one", "is_dir": false},
				map[string]any{"name": "two", "path": "/workspace/two", "is_dir": false},
			},
			"limit": 1, "offset": 0, "has_previous": false, "previous_offset": 0,
			"has_more": true, "next_offset": 1,
		})
	}))
	t.Cleanup(server.Close)
	if _, err := NewClient(server.URL, "", 0).Browse(
		context.Background(), "/workspace", BrowseOptions{Limit: 1},
	); err == nil {
		t.Fatal("oversized host bridge page was accepted")
	}
}

func TestClientBrowseRejectsMalformedEntryInsteadOfSkippingIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": "/workspace", "entries": []any{"not-an-entry"},
			"limit": 1, "offset": 0, "has_previous": false, "previous_offset": 0,
			"has_more": false, "next_offset": 0,
		})
	}))
	t.Cleanup(server.Close)
	if _, err := NewClient(server.URL, "", 0).Browse(
		context.Background(), "/workspace", BrowseOptions{Limit: 1},
	); err == nil {
		t.Fatal("malformed host bridge entry was silently skipped")
	}
}

func TestClientBrowseRejectsStringBooleanEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": "/workspace", "entries": []any{
				map[string]any{"name": "one", "path": "/workspace/one", "is_dir": "true"},
			},
			"limit": 1, "offset": 0, "has_previous": false, "previous_offset": 0,
			"has_more": false, "next_offset": 0,
		})
	}))
	t.Cleanup(server.Close)
	if _, err := NewClient(server.URL, "", 0).Browse(
		context.Background(), "/workspace", BrowseOptions{Limit: 1},
	); err == nil {
		t.Fatal("string host bridge entry boolean was coerced")
	}
}
