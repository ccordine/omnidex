package api

import (
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCharacterEditorRouteProjectsExactCharacter(t *testing.T) {
	t.Parallel()
	simulation := roleplayCharacterEditorHTTPStore()
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(http.MethodGet,
		"/v1/ui/roleplay/character?channel_id=story-http&character_id="+roleplayHTTPActive, nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("cache-control=%q", cache)
	}
	var payload roleplayCharacterEditorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ChannelID != roleplayHTTPChannelID || payload.WorldID != simulation.world.ID ||
		payload.CharacterID != roleplayHTTPActive {
		t.Fatalf("payload authority=%+v", payload)
	}
	for _, required := range []string{
		"Active Archivist", `data-roleplay-character-editor-character="` + roleplayHTTPActive + `"`,
		`saveRoleplayCharacterPersona`, `saveRoleplayCharacterGeneration`, `saveRoleplayCharacterResearch`,
	} {
		if !strings.Contains(payload.HTML.Bundle, required) {
			t.Errorf("editor response lacks %q: %s", required, payload.HTML.Bundle)
		}
	}
}

func TestRoleplayCharacterEditorRouteRejectsInexactAndForeignAuthority(t *testing.T) {
	t.Parallel()
	simulation := roleplayCharacterEditorHTTPStore()
	foreignID := "rpc_cccccccccccccccccccccccccccccccc"
	simulation.names[foreignID] = "Foreign"
	simulation.characterWorldIDs[foreignID] = "rpw_cccccccccccccccccccccccccccccccc"
	server := newRoleplayHTTPTestServer(t, simulation)
	tests := []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/v1/ui/roleplay/character?channel_id=story-http", http.StatusBadRequest},
		{http.MethodGet, "/v1/ui/roleplay/character?channel_id=story-http&character_id=bad", http.StatusBadRequest},
		{http.MethodGet, "/v1/ui/roleplay/character?channel_id=story-http&character_id=" + roleplayHTTPActive + "&extra=1", http.StatusBadRequest},
		{http.MethodGet, "/v1/ui/roleplay/character?channel_id=story-http&channel_id=other&character_id=" + roleplayHTTPActive, http.StatusBadRequest},
		{http.MethodGet, "/v1/ui/roleplay/character?channel_id=story-http&character_id=" + foreignID, http.StatusNotFound},
		{http.MethodPost, "/v1/ui/roleplay/character?channel_id=story-http&character_id=" + roleplayHTTPActive, http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("method=%s path=%s status=%d want=%d body=%s",
				test.method, test.path, response.Code, test.status, response.Body.String())
		}
	}
}

func TestRoleplayCharacterEditorRouteRendersMissingPersonaWithoutFallback(t *testing.T) {
	t.Parallel()
	simulation := roleplayCharacterEditorHTTPStore()
	delete(simulation.personaConfigured, roleplayHTTPActive)
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(http.MethodGet,
		"/v1/ui/roleplay/character?channel_id=story-http&character_id="+roleplayHTTPActive, nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Sheet required") ||
		!strings.Contains(response.Body.String(), `name=\"expected_revision\" value=\"0\"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRoleplayCharacterEditorRouteKeepsUnavailableConfiguredModel(t *testing.T) {
	t.Parallel()
	simulation := roleplayCharacterEditorHTTPStore()
	generation := simulation.generation[roleplayHTTPActive]
	generation.Config.NarrativeModel = "removed-model:9b"
	simulation.generation[roleplayHTTPActive] = generation
	server := newRoleplayHTTPTestServer(t, simulation)
	server.ollamaModelAuthority = &roleplayModelPageProbe{pages: map[int]ollama.ModelPage{
		0: {Offset: 0, Models: []ollama.ModelInfo{{Name: "replacement-model:4b"}}},
	}}
	request := httptest.NewRequest(http.MethodGet,
		"/v1/ui/roleplay/character?channel_id=story-http&character_id="+roleplayHTTPActive, nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	var payload roleplayCharacterEditorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	body := html.UnescapeString(payload.HTML.Bundle)
	if response.Code != http.StatusOK ||
		!strings.Contains(body, `<option value="removed-model:9b" selected disabled>`) ||
		!strings.Contains(body, `<option value="replacement-model:4b">replacement-model:4b</option>`) {
		t.Fatalf("status=%d body=%s", response.Code, body)
	}
}

func TestRoleplayCharacterEditorRouteMapsModelCatalogFailureToBadGateway(t *testing.T) {
	t.Parallel()
	server := newRoleplayHTTPTestServer(t, roleplayCharacterEditorHTTPStore())
	server.ollamaModelAuthority = &roleplayModelPageProbe{err: errors.New("catalog transport failed")}
	request := httptest.NewRequest(http.MethodGet,
		"/v1/ui/roleplay/character?channel_id=story-http&character_id="+roleplayHTTPActive, nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway ||
		!strings.Contains(response.Body.String(), "roleplay model catalog is unavailable") ||
		!strings.Contains(response.Body.String(), "catalog transport failed") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func roleplayCharacterEditorHTTPStore() *roleplaySimulationTestStore {
	store := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	store.names[roleplayHTTPActive] = "Active Archivist"
	store.personaConfigured[roleplayHTTPActive] = true
	store.personas.Items = []roleplay.PersonaProjection{{
		CharacterID: roleplayHTTPActive, Revision: 2,
		Sheet: roleplay.PersonaSheet{Summary: "Keeps exact test authority."}, UpdatedAt: time.Now().UTC(),
	}}
	store.generation[roleplayHTTPActive] = roleplay.CharacterGenerationProjection{
		CharacterID: roleplayHTTPActive,
		Config: roleplay.CharacterGenerationConfig{
			Schema:             roleplay.CharacterGenerationConfigSchemaV2,
			LibraryCharacterID: "rpl_11111111111111111111111111111111", Revision: 3,
		},
		UpdatedAt: time.Now().UTC(),
	}
	return store
}
