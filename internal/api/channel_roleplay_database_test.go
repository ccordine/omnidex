package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestRoleplayChannelCreateEndpointAtomicallyBootstrapsPersistedAuthority(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	server := NewServer(nil, nil)
	server.repo = repository
	server.channelStore = repository
	server.mux = http.NewServeMux()
	server.routes()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/channels",
		bytes.NewBufferString(`{"id":"api-harbor-story","name":"Harbor story","tags":["user-channel"],"workspace_root":"/srv/workspaces/api-harbor-story","mode":"roleplay","roleplay_world_name":"Harbor Kingdom","roleplay_viewpoint_name":"Alice"}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Channel model.Channel `json:"channel"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Channel.Mode != model.ChannelModeRoleplay || payload.Channel.DataSourceID != "" {
		t.Fatalf("channel=%+v", payload.Channel)
	}
	if err := payload.Channel.RoleplayViewpointCharacterID.Validate(); err != nil {
		t.Fatalf("server viewpoint=%q: %v", payload.Channel.RoleplayViewpointCharacterID, err)
	}
	var worldName, viewpointName, viewpointID string
	if err := pool.QueryRow(t.Context(), `
		SELECT world.name, character.name, character.id
		FROM roleplay_worlds AS world
		JOIN roleplay_characters AS character ON character.world_id=world.id
		WHERE world.channel_id=$1
	`, payload.Channel.ID).Scan(&worldName, &viewpointName, &viewpointID); err != nil {
		t.Fatal(err)
	}
	if worldName != "Harbor Kingdom" || viewpointName != "Alice" ||
		viewpointID != string(payload.Channel.RoleplayViewpointCharacterID) {
		t.Fatalf("world=%q viewpoint=%q id=%q channel=%+v", worldName, viewpointName, viewpointID, payload.Channel)
	}
	for _, forbidden := range []string{"roleplay_world_name", "roleplay_viewpoint_name", "Harbor Kingdom", "Alice"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("channel response leaked creation-only authority %q: %s", forbidden, response.Body.String())
		}
	}
}
