package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	roleplayHTTPChannelID = "story-http"
	roleplayHTTPViewpoint = "rpc_00000000000000000000000000000000"
	roleplayHTTPActive    = "rpc_11111111111111111111111111111111"
)

func TestRoleplaySimulationRouteFailsLoudlyWithoutInjectedStore(t *testing.T) {
	t.Parallel()
	server := newRoleplayHTTPTestServer(t, nil)
	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/v1/ui/chat/roleplay?channel_id=story-http", nil),
		httptest.NewRequest(
			http.MethodPost, "/v1/channels/story-http/roleplay/characters", bytes.NewBufferString(`{"name":"Signal Keeper"}`),
		),
	}
	for _, request := range requests {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "store is unavailable") {
			t.Fatalf("method=%s status=%d body=%s", request.Method, response.Code, response.Body.String())
		}
	}
}

func TestRoleplaySimulationComponentUsesSQLPagesAndActiveSceneCharacter(t *testing.T) {
	t.Parallel()
	simulation := configuredRoleplayHTTPTestStore()
	server := newRoleplayHTTPTestServer(t, simulation)
	query := "channel_id=story-http&characters_offset=4&personas_offset=4&turn_order_offset=4" +
		"&meters_offset=4&inventory_offset=4&interactions_offset=4&item_templates_offset=4"
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/chat/roleplay?"+query, nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if simulation.meterCharacterID != roleplayHTTPActive || simulation.inventoryCharacterID != roleplayHTTPActive {
		t.Fatalf("meter_character=%q inventory_character=%q", simulation.meterCharacterID, simulation.inventoryCharacterID)
	}
	if simulation.characterOffset != 4 || simulation.personaOffset != 4 || simulation.participantOffset != 4 ||
		simulation.meterOffset != 4 || simulation.inventoryOffset != 4 || simulation.interactionOffset != 4 ||
		simulation.itemTemplateOffset != 4 {
		t.Fatalf("server offsets were not passed to PostgreSQL pages: %+v", simulation)
	}
	if simulation.snapshotCalls != 1 || simulation.configuredPageCalls != 0 {
		t.Fatalf("snapshot_calls=%d legacy_page_calls=%d", simulation.snapshotCalls, simulation.configuredPageCalls)
	}
	var payload roleplaySimulationComponentResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Configured || payload.SceneRevision == nil || *payload.SceneRevision != 9 ||
		!strings.Contains(payload.HTML.Bundle, "Current turn: Active Archivist") {
		t.Fatalf("payload=%+v body=%s", payload, response.Body.String())
	}
}

func TestRoleplaySimulationUnconfiguredStateDoesNotQueryScenePages(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	simulation.characters.Items = []roleplay.SimulationCharacterSummary{{
		ID: roleplayHTTPViewpoint, WorldID: simulation.world.ID, Name: "Initial Navigator", CreatedAt: time.Now().UTC(),
	}}
	simulation.personaConfigured[roleplayHTTPViewpoint] = true
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(http.MethodGet, "/v1/ui/chat/roleplay?channel_id=story-http", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || simulation.snapshotCalls != 1 || simulation.configuredPageCalls != 0 {
		t.Fatalf("status=%d snapshots=%d configured_calls=%d body=%s", response.Code, simulation.snapshotCalls, simulation.configuredPageCalls, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "Simulation setup required") ||
		!strings.Contains(response.Body.String(), `saveRoleplaySceneDraftParticipant`) {
		t.Fatalf("unconfigured setup is absent: %s", response.Body.String())
	}
}

func TestRoleplayCharacterCreateIssuesIdentityAndReconcilesServerComponent(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/channels/story-http/roleplay/characters",
		bytes.NewBufferString(`{"name":"Orchid Cartographer"}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated || simulation.characterCreateCalls != 1 ||
		!strings.Contains(response.Body.String(), "Orchid Cartographer") {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, simulation.characterCreateCalls, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "rpc_cccccccccccccccccccccccccccccccc</") {
		t.Fatalf("component exposed an opaque identity as user-facing text: %s", response.Body.String())
	}
}

func TestRoleplayConfigRejectsSecondActionTransportAndInexactBodies(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	server := newRoleplayHTTPTestServer(t, simulation)
	tests := []struct {
		path string
		body string
		want int
	}{
		{"/v1/channels/story-http/roleplay/actions", `{"command":"/give \"lens\""}`, http.StatusNotFound},
		{"/v1/channels/story-http/roleplay/research", `{"question":"run this now"}`, http.StatusNotFound},
		{"/v1/channels/story-http/roleplay/characters", `{"name":"Exact","id":"rpc_22222222222222222222222222222222"}`, http.StatusBadRequest},
		{"/v1/channels/story-http/roleplay/characters", `{"name":"Exact","name":"Duplicate"}`, http.StatusBadRequest},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewBufferString(test.body))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("path=%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
	if simulation.characterCreateCalls != 0 {
		t.Fatalf("invalid transports mutated character authority: %d", simulation.characterCreateCalls)
	}
}

func newRoleplayHTTPTestServer(t *testing.T, simulation RoleplaySimulationStore) *Server {
	t.Helper()
	channels := newChannelTestStore()
	now := time.Now().UTC()
	channels.channels[roleplayHTTPChannelID] = model.Channel{
		ID: roleplayHTTPChannelID, Scope: model.ChannelScopeUser, Name: "Story HTTP", Tags: []string{"user-channel"},
		ProjectID: 42, WorkspaceRoot: "/workspace/story-http", Mode: model.ChannelModeRoleplay,
		RoleplayViewpointCharacterID: roleplayHTTPViewpoint, CreatedAt: now, UpdatedAt: now,
	}
	server := NewServerWithOptions(nil, nil, ServerOptions{RoleplaySimulation: simulation})
	server.channelStore = channels
	server.mux = http.NewServeMux()
	server.routes()
	return server
}

func configuredRoleplayHTTPTestStore() *roleplaySimulationTestStore {
	store := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	now := time.Now().UTC()
	store.scene = &roleplay.SceneSheet{
		ID: "rps_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", WorldID: store.world.ID,
		Title: "Archive Shift", Description: "A task-neutral scene.", Revision: 9,
		ActiveCharacterID: roleplayHTTPActive, CreatedAt: now, UpdatedAt: now,
	}
	store.characters.Items = []roleplay.SimulationCharacterSummary{{
		ID: roleplayHTTPActive, WorldID: store.world.ID, Name: "Active Archivist", CreatedAt: now,
	}}
	store.names[roleplayHTTPActive] = "Active Archivist"
	store.personaConfigured[roleplayHTTPActive] = true
	store.personas.Items = []roleplay.PersonaProjection{{
		CharacterID: roleplayHTTPActive, Revision: 2,
		Sheet: roleplay.PersonaSheet{Summary: "Maps signals", Voice: "Precise"}, UpdatedAt: now,
	}}
	store.participants.Items = []roleplay.SceneParticipantProjection{{
		CharacterID: roleplayHTTPActive, Name: "Active Archivist", TurnPosition: 4,
	}}
	store.allParticipants = []roleplay.SceneParticipantProjection{{
		CharacterID: roleplayHTTPActive, Name: "Active Archivist", TurnPosition: 0,
	}}
	store.meters.Items = []roleplay.MeterProjection{{
		Key: "signal", Name: "Signal", Minimum: 0, Maximum: 10, Value: 4, Revision: 3,
	}}
	store.inventory.Items = []roleplay.InventoryItemProjection{{
		ID: "rpv_dddddddddddddddddddddddddddddddd", TemplateID: "rpi_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Name: "Cipher Lens", Description: "Configured inventory.", UsePolicy: roleplay.ItemUseInfinite,
	}}
	store.interactions.Items = []roleplay.InteractionCommandDefinition{{
		ID: "rpa_ffffffffffffffffffffffffffffffff", WorldID: store.world.ID,
		Key: "calibrate", Name: "Calibrate", Description: "Adjust a signal.", ArgumentMode: roleplay.CommandArgumentRequired,
	}}
	store.itemTemplates.Items = []roleplay.ItemTemplateDefinition{{
		ID: "rpi_abababababababababababababababab", WorldID: store.world.ID,
		Name: "Cipher Lens", Description: "Reads a signal.", UsePolicy: roleplay.ItemUseFinite,
		InitialUses: 2, Priority: 4, Effects: []roleplay.MeterDelta{{MeterKey: "signal", Delta: -1}},
	}}
	return store
}
