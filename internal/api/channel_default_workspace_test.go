package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestChannelCreationResolvesOmittedWorkspaceFromExactServerDefault(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	server.providerConfig.WorkspaceRoot = "/srv/workspaces/server-default"

	response := createChannelTestRequest(t, server,
		`{"id":"default-chat","name":"Default chat","tags":["user-channel"],"mode":"assistant"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Channel model.Channel `json:"channel"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	stored := store.channels["default-chat"]
	if stored.WorkspaceRoot != server.providerConfig.WorkspaceRoot ||
		payload.Channel.WorkspaceRoot != server.providerConfig.WorkspaceRoot {
		t.Fatalf("stored workspace=%q response workspace=%q default=%q",
			stored.WorkspaceRoot, payload.Channel.WorkspaceRoot, server.providerConfig.WorkspaceRoot)
	}
}

func TestChannelCreationPrefersExactExplicitWorkspaceOverServerDefault(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	server.providerConfig.WorkspaceRoot = "/srv/workspaces/server-default"

	response := createChannelTestRequest(t, server,
		`{"id":"explicit-chat","name":"Explicit chat","tags":[],"workspace_root":"/srv/workspaces/explicit","mode":"assistant"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stored := store.channels["explicit-chat"]; stored.WorkspaceRoot != "/srv/workspaces/explicit" {
		t.Fatalf("stored workspace=%q", stored.WorkspaceRoot)
	}
}

func TestChannelCreationRejectsExplicitInvalidWorkspaceWithoutDefaulting(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	server.providerConfig.WorkspaceRoot = "/srv/workspaces/server-default"
	for _, workspace := range []string{
		`null`, `""`, `" relative"`, `"relative/path"`, `"/srv/workspaces/../other"`, `42`,
	} {
		body := `{"id":"invalid-workspace","name":"Invalid workspace","tags":[],"workspace_root":` + workspace + `,"mode":"assistant"}`
		response := createChannelTestRequest(t, server, body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("workspace=%s status=%d body=%s", workspace, response.Code, response.Body.String())
		}
	}
	if _, exists := store.channels["invalid-workspace"]; exists {
		t.Fatal("invalid explicit workspace mutated durable channel state")
	}
}

func TestChannelCreationFailsLoudlyWhenServerDefaultWorkspaceIsInvalid(t *testing.T) {
	t.Parallel()
	for _, configured := range []string{"", "relative/path", "/srv/workspaces/../other", " /srv/workspaces/default"} {
		configured := configured
		t.Run(configured, func(t *testing.T) {
			t.Parallel()
			server, store := newChannelFrontdoorTestServer(t)
			server.providerConfig.WorkspaceRoot = configured
			response := createChannelTestRequest(t, server,
				`{"id":"missing-default","name":"Missing default","tags":[],"mode":"assistant"}`)
			if response.Code != http.StatusServiceUnavailable ||
				!strings.Contains(response.Body.String(), "server default channel workspace root is invalid") {
				t.Fatalf("configured=%q status=%d body=%s", configured, response.Code, response.Body.String())
			}
			if _, exists := store.channels["missing-default"]; exists {
				t.Fatal("invalid server default mutated durable channel state")
			}
		})
	}
}

func TestRoleplayChannelCreationAlsoUsesServerDefaultWorkspace(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	server.providerConfig.WorkspaceRoot = "/srv/workspaces/story-default"
	response := createChannelTestRequest(t, server,
		`{"id":"default-story","name":"Default story","tags":[],"mode":"roleplay","roleplay_world_name":"Harbor","roleplay_viewpoint_name":"Alice"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	stored := store.channels["default-story"]
	if stored.WorkspaceRoot != server.providerConfig.WorkspaceRoot || stored.Mode != model.ChannelModeRoleplay {
		t.Fatalf("stored channel=%+v", stored)
	}
}

func createChannelTestRequest(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/channels", bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
