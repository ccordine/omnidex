package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplaySceneDraftPersistsFiveOrderedParticipantsAcrossPagesRefreshAndTabs(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	characters := make([]roleplay.SimulationCharacterSummary, 6)
	for index := range characters {
		id := "rpc_" + strings.Repeat(strconv.FormatInt(int64(index+1), 16), 32)
		characters[index] = roleplay.SimulationCharacterSummary{
			ID: id, WorldID: simulation.world.ID, Name: "Participant " + strconv.Itoa(index+1), CreatedAt: time.Now().UTC(),
		}
		simulation.names[id] = characters[index].Name
		simulation.personaConfigured[id] = true
	}
	simulation.characters = roleplay.SimulationCharacterPage{Items: characters[:4], HasMore: true}
	simulation.worldCharacters = append([]roleplay.SimulationCharacterSummary(nil), characters...)
	server := newRoleplayHTTPTestServer(t, simulation)
	initial := httptest.NewRecorder()
	server.Handler().ServeHTTP(initial, httptest.NewRequest(
		http.MethodGet, "/v1/ui/chat/roleplay?channel_id=story-http", nil,
	))
	if initial.Code != http.StatusOK || len(initial.Result().Cookies()) != 1 {
		t.Fatalf("initial status=%d cookies=%d body=%s", initial.Code, len(initial.Result().Cookies()), initial.Body.String())
	}
	cookie := initial.Result().Cookies()[0]
	for index := 0; index < 5; index++ {
		offset := 0
		if index == 4 {
			offset = 4
			simulation.characters = roleplay.SimulationCharacterPage{Items: characters[4:]}
		} else {
			simulation.characters = roleplay.SimulationCharacterPage{Items: characters[:4], HasMore: true}
		}
		body := `{"expected_revision":` + strconv.Itoa(index) + `,"selected":true,"characters_offset":` + strconv.Itoa(offset) + `}`
		request := httptest.NewRequest(
			http.MethodPut,
			"/v1/channels/story-http/roleplay/scene-draft/participants/"+characters[index].ID,
			bytes.NewBufferString(body),
		)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("selection %d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}
	simulation.characters = roleplay.SimulationCharacterPage{Items: characters[:4], HasMore: true}
	for _, label := range []string{"refresh", "second tab"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/ui/chat/roleplay?channel_id=story-http", nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || strings.Count(response.Body.String(), `name=\"participant_id\"`) != 5 ||
			!strings.Contains(response.Body.String(), "Server draft turn order · 5 selected") {
			t.Fatalf("%s did not restore five-person server draft: status=%d body=%s", label, response.Code, response.Body.String())
		}
	}
	participantIDs := make([]string, 5)
	for index := range participantIDs {
		participantIDs[index] = characters[index].ID
	}
	payload, err := json.Marshal(map[string]any{
		"expected_draft_revision": 5,
		"title":                   "Five-person scene", "description": "Ordered across two server pages.",
		"participant_ids": participantIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/channels/story-http/roleplay/scene", bytes.NewReader(payload))
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !reflect.DeepEqual(simulation.lastSceneSetup.ParticipantIDs, participantIDs) {
		t.Fatalf("scene status=%d setup=%+v body=%s", response.Code, simulation.lastSceneSetup, response.Body.String())
	}

	for index, change := range []struct {
		character int
		selected  bool
		offset    int
	}{
		{character: 0, selected: false, offset: 0},
		{character: 5, selected: true, offset: 4},
	} {
		if change.offset == 0 {
			simulation.characters = roleplay.SimulationCharacterPage{Items: characters[:4], HasMore: true}
		} else {
			simulation.characters = roleplay.SimulationCharacterPage{Items: characters[4:]}
		}
		payload := `{"expected_revision":` + strconv.Itoa(6+index) + `,"selected":` +
			strconv.FormatBool(change.selected) + `,"characters_offset":` + strconv.Itoa(change.offset) + `}`
		request := httptest.NewRequest(
			http.MethodPut,
			"/v1/channels/story-http/roleplay/scene-draft/participants/"+characters[change.character].ID,
			bytes.NewBufferString(payload),
		)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("configured selection %d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}
	updatedParticipantIDs := make([]string, 5)
	for index := range updatedParticipantIDs {
		updatedParticipantIDs[index] = characters[index+1].ID
	}
	updatePayload, err := json.Marshal(map[string]any{
		"expected_revision": 1, "expected_draft_revision": 8,
		"title": "Reordered scene", "description": "Configured across two server pages.",
		"participant_ids": updatedParticipantIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	updateRequest := httptest.NewRequest(
		http.MethodPut, "/v1/channels/story-http/roleplay/scene", bytes.NewReader(updatePayload),
	)
	updateRequest.AddCookie(cookie)
	updateResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK ||
		!reflect.DeepEqual(simulation.lastSceneUpdate.ParticipantIDs, updatedParticipantIDs) {
		t.Fatalf("update status=%d update=%+v body=%s", updateResponse.Code, simulation.lastSceneUpdate, updateResponse.Body.String())
	}
}

func TestRoleplaySceneUpdateRejectsStaleRevisionWithoutMutation(t *testing.T) {
	t.Parallel()
	simulation := configuredRoleplayHTTPTestStore()
	simulation.participants.Items[0].TurnPosition = 0
	simulation.sceneUpdateErr = roleplay.ErrSimulationStaleRevision
	server := newRoleplayHTTPTestServer(t, simulation)
	originalTitle := simulation.scene.Title
	body := `{"expected_revision":8,"title":"Stale title","description":"Must not persist.","participant_ids":["` + roleplayHTTPActive + `"]}`
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(
		http.MethodPut, "/v1/channels/story-http/roleplay/scene", bytes.NewBufferString(body),
	))
	if response.Code != http.StatusConflict || simulation.scene.Title != originalTitle {
		t.Fatalf("status=%d scene=%+v body=%s", response.Code, simulation.scene, response.Body.String())
	}
	refreshed := httptest.NewRecorder()
	server.Handler().ServeHTTP(refreshed, httptest.NewRequest(
		http.MethodGet, "/v1/ui/chat/roleplay?channel_id=story-http", nil,
	))
	if refreshed.Code != http.StatusOK || !strings.Contains(refreshed.Body.String(), originalTitle) ||
		strings.Contains(refreshed.Body.String(), "Stale title") {
		t.Fatalf("refreshed status=%d body=%s", refreshed.Code, refreshed.Body.String())
	}
}
