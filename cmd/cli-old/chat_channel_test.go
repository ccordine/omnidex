package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestChatSessionOwnsOneDeterministicCanonicalChannel(t *testing.T) {
	t.Parallel()
	first, err := chatChannelForSession("project-session", "/work/project")
	if err != nil {
		t.Fatal(err)
	}
	second, err := chatChannelForSession("project-session", "/work/project")
	if err != nil {
		t.Fatal(err)
	}
	other, err := chatChannelForSession("another-session", "/work/project")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.ID == other.ID {
		t.Fatalf("channel mapping first=%q second=%q other=%q", first.ID, second.ID, other.ID)
	}
	if first.Scope != model.ChannelScopeUser || first.Name == "" || len(first.Tags) == 0 {
		t.Fatalf("channel=%+v", first)
	}
	if err := first.ValidateForCreate(); err != nil {
		t.Fatal(err)
	}
}

func TestChatSessionChannelRejectsBlankSession(t *testing.T) {
	t.Parallel()
	if _, err := chatChannelForSession(" \n\t ", "/work/project"); err == nil {
		t.Fatal("blank session was accepted")
	}
}

func TestChatSessionRejectsAStoredChannelBoundToAnotherWorkspace(t *testing.T) {
	t.Parallel()
	expected, err := chatChannelForSession("project-session", "/work/project")
	if err != nil {
		t.Fatal(err)
	}
	stored := expected
	stored.ProjectID = 42
	stored.WorkspaceRoot = "/work/another-project"
	if _, err := validateChatSessionChannel(expected, stored); err == nil {
		t.Fatal("CLI chat reused a session channel bound to another workspace")
	}
}

func TestEnsureChatChannelCreatesOnceThenReusesServerAuthority(t *testing.T) {
	t.Parallel()
	expected, err := chatChannelForSession("project-session", "/work/project")
	if err != nil {
		t.Fatal(err)
	}
	stored := expected
	stored.ProjectID = 42
	created := false
	gets := 0
	creates := 0
	var stateMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		stateMu.Lock()
		defer stateMu.Unlock()
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/channels/"+string(expected.ID):
			gets++
			if !created {
				response.WriteHeader(http.StatusNotFound)
				_, _ = response.Write([]byte(`{"error":"channel not found"}`))
				return
			}
			writeChatChannelTestJSON(response, http.StatusOK, map[string]any{"channel": stored})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/channels":
			creates++
			var payload struct {
				ID            model.ChannelID `json:"id"`
				Name          string          `json:"name"`
				Tags          []string        `json:"tags"`
				WorkspaceRoot string          `json:"workspace_root"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.ID != expected.ID || payload.Name != expected.Name || len(payload.Tags) != 2 ||
				payload.WorkspaceRoot != expected.WorkspaceRoot {
				http.Error(response, "create payload does not match expected channel", http.StatusBadRequest)
				return
			}
			created = true
			writeChatChannelTestJSON(response, http.StatusCreated, map[string]any{"channel": stored})
		default:
			http.Error(response, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	apiClient := client.New(server.URL, 0)
	first, err := ensureChatChannel(context.Background(), apiClient, "project-session", "/work/project")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureChatChannel(context.Background(), apiClient, "project-session", "/work/project")
	if err != nil {
		t.Fatal(err)
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	if first.ID != expected.ID || second.ID != expected.ID || gets != 2 || creates != 1 {
		t.Fatalf("first=%+v second=%+v gets=%d creates=%d", first, second, gets, creates)
	}
}

func writeChatChannelTestJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(payload); err != nil {
		panic(err)
	}
}
