package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayWorkspaceHTTPListsAndCreatesLibraryCharacters(t *testing.T) {
	store := newRoleplaySimulationTestStore("story-world")
	store.library.Items = []roleplay.LibraryCharacterSummary{{
		ID: "rpl_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Name: "Mira",
		Authority: roleplay.AuthorityCharacterIdentity, PlacedInSelectedWorld: true,
	}}
	server := NewServerWithOptions(nil, nil, ServerOptions{RoleplaySimulation: store})

	worlds := httptest.NewRecorder()
	server.Handler().ServeHTTP(worlds, httptest.NewRequest(http.MethodGet, "/v1/ui/roleplay/worlds?limit=20&offset=0", nil))
	if worlds.Code != http.StatusOK || !strings.Contains(worlds.Body.String(), "roleplay-world-list") {
		t.Fatalf("worlds status=%d body=%s", worlds.Code, worlds.Body.String())
	}

	library := httptest.NewRecorder()
	server.Handler().ServeHTTP(library, httptest.NewRequest(
		http.MethodGet, "/v1/ui/roleplay/library?limit=20&offset=0&channel_id=story-world", nil,
	))
	if library.Code != http.StatusOK || store.libraryWorldID != store.world.ID ||
		!strings.Contains(library.Body.String(), "In world") ||
		strings.Contains(library.Body.String(), `chat#placeRoleplayCharacter`) {
		t.Fatalf("library status=%d world=%q body=%s", library.Code, store.libraryWorldID, library.Body.String())
	}

	created := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/ui/roleplay/library?channel_id=story-world", strings.NewReader(`{"name":"Mira"}`))
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(created, request)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), "Mira") ||
		!strings.Contains(created.Body.String(), "roleplay-library-list") {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}

	unknown := httptest.NewRecorder()
	badRequest := httptest.NewRequest(http.MethodPost, "/v1/ui/roleplay/library", strings.NewReader(`{"name":"Mira","unknown":true}`))
	badRequest.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(unknown, badRequest)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", unknown.Code, unknown.Body.String())
	}

	missing := httptest.NewRecorder()
	server.Handler().ServeHTTP(missing, httptest.NewRequest(
		http.MethodGet, "/v1/ui/roleplay/library?channel_id=missing-world", nil,
	))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing world status=%d body=%s", missing.Code, missing.Body.String())
	}
}
