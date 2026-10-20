package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionlist"
)

func TestCLISessionsRejectsForeignDuplicateAndMalformedObservations(t *testing.T) {
	identity := "client_" + strings.Repeat("1", 32) + "_directory_1_101"
	entry := sessionlist.Entry{ID: model.ChannelID("cli-chat-" + strings.Repeat("1", 32)), Directory: "/client/one", WorkspaceIdentity: identity, Activity: "running", ObservedAt: time.Now()}
	for _, name := range []string{"valid", "foreign", "duplicate", "invalid status", "missing array", "invalid continuation", "oversized page"} {
		t.Run(name, func(t *testing.T) {
			listing := sessionlist.Listing{Sessions: []sessionlist.Entry{entry}}
			switch name {
			case "foreign":
				listing.Sessions[0].WorkspaceIdentity = "client_" + strings.Repeat("2", 32) + "_directory_1_101"
			case "duplicate":
				listing.Sessions = append(listing.Sessions, entry)
			case "invalid status":
				listing.Sessions[0].Activity = "probably working"
			case "invalid continuation":
				listing.NextCursor = model.ChannelID("cli-chat-" + strings.Repeat("2", 32))
			case "oversized page":
				for len(listing.Sessions) <= sessionlist.PageSize {
					listing.Sessions = append(listing.Sessions, entry)
				}
			case "missing array":
				listing.Sessions = nil
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/channels/cli-sessions" || r.URL.Query().Get("workspace_identity") != identity || r.URL.Query().Get("workspace_root") != "/client/one" {
					t.Error("listing request lost exact origin")
				}
				json.NewEncoder(w).Encode(listing)
			}))
			defer server.Close()
			apiClient, err := New(server.URL, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = apiClient.CLISessions(t.Context(), "/client/one", identity)
			if (err == nil) != (name == "valid") {
				t.Fatal("listing validation", err)
			}
		})
	}
}

func TestCLISessionsPaginatesBeyondResponseLimit(t *testing.T) {
	identity := "client_" + strings.Repeat("1", 32) + "_directory_1_101"
	const total = 3000
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		start := 0
		if cursor := r.URL.Query().Get("after"); cursor != "" {
			if _, err := fmt.Sscanf(cursor, "cli-chat-%032x", &start); err != nil {
				t.Error(err)
			}
		}
		page := sessionlist.Listing{Sessions: []sessionlist.Entry{}}
		end := min(start+sessionlist.PageSize, total)
		for i := start + 1; i <= end; i++ {
			page.Sessions = append(page.Sessions, sessionlist.Entry{ID: model.ChannelID(fmt.Sprintf("cli-chat-%032x", i)), Directory: "/" + strings.Repeat("a", 3000), WorkspaceIdentity: identity, Activity: "running", ObservedAt: time.Now()})
		}
		if end < total {
			page.NextCursor = page.Sessions[len(page.Sessions)-1].ID
		}
		json.NewEncoder(w).Encode(page)
	}))
	defer server.Close()
	c, err := New(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	listing, err := c.CLISessions(t.Context(), "/client/one", identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Sessions) != total || requests != 30 {
		t.Fatalf("sessions=%d requests=%d", len(listing.Sessions), requests)
	}
}
