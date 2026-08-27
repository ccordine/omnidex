package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplaySceneCreateUsesServerIdentityAndExactSelectedOrder(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	first := roleplayHTTPViewpoint
	second := roleplayHTTPActive
	now := time.Now().UTC()
	simulation.characters.Items = []roleplay.SimulationCharacterSummary{
		{ID: first, WorldID: simulation.world.ID, Name: "North Cartographer", CreatedAt: now},
		{ID: second, WorldID: simulation.world.ID, Name: "Signal Keeper", CreatedAt: now},
	}
	simulation.worldCharacters = append([]roleplay.SimulationCharacterSummary(nil), simulation.characters.Items...)
	simulation.names[first], simulation.names[second] = "North Cartographer", "Signal Keeper"
	simulation.personaConfigured[first], simulation.personaConfigured[second] = true, true
	server := newRoleplayHTTPTestServer(t, simulation)
	body := `{"title":"Signal Room","description":"A bounded unrelated scene.","participant_ids":["` +
		second + `","` + first + `"]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/channels/story-http/roleplay/scene", bytes.NewBufferString(body))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.HasPrefix(simulation.lastSceneSetup.ID, "rps_") || simulation.lastSceneSetup.WorldID != simulation.world.ID ||
		!reflect.DeepEqual(simulation.lastSceneSetup.ParticipantIDs, []string{second, first}) {
		t.Fatalf("scene setup=%+v", simulation.lastSceneSetup)
	}
	if simulation.lastSceneSetup.ParticipantIDs[0] != simulation.scene.ActiveCharacterID {
		t.Fatalf("active character did not follow exact selected order: %+v", simulation.scene)
	}
}

func TestRoleplayInlineIdentityCreationAtomicallyAddsSelectableSceneParticipant(t *testing.T) {
	t.Parallel()
	simulation := configuredRoleplayHTTPTestStore()
	simulation.participants.Items[0].TurnPosition = 0
	server := newRoleplayHTTPTestServer(t, simulation)
	modelCatalog := &roleplayModelPageProbe{err: errors.New("component projection unavailable")}
	server.ollamaModelAuthority = modelCatalog
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/channels/story-http/roleplay/user-personas",
		bytes.NewBufferString(`{"name":"Orchid Cartographer"}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload roleplayUserPersonaCreationReceipt
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	createdID := payload.CharacterID
	if createdID != "rpc_cccccccccccccccccccccccccccccccc" ||
		payload.ChannelID != roleplayHTTPChannelID ||
		len(simulation.allParticipants) != 2 ||
		simulation.allParticipants[1].CharacterID != createdID ||
		simulation.scene.Revision != 10 {
		t.Fatalf("created=%q scene=%+v participants=%+v", createdID, simulation.scene, simulation.allParticipants)
	}
	if response.Body.String() != `{"channel_id":"story-http","character_id":"rpc_cccccccccccccccccccccccccccccccc"}`+"\n" {
		t.Fatalf("creation receipt was not exact: %s", response.Body.String())
	}
	if simulation.snapshotCalls != 0 || len(modelCatalog.offsets) != 0 {
		t.Fatalf("committed creation invoked fallible projection: snapshots=%d model_offsets=%v",
			simulation.snapshotCalls, modelCatalog.offsets)
	}
	failedProjectionRequest := httptest.NewRequest(
		http.MethodGet,
		"/v1/ui/chat/roleplay?channel_id=story-http&composer_persona_character_id="+createdID,
		nil,
	)
	failedProjectionResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(failedProjectionResponse, failedProjectionRequest)
	if failedProjectionResponse.Code != http.StatusBadGateway ||
		len(simulation.allParticipants) != 2 || simulation.allParticipants[1].CharacterID != createdID {
		t.Fatalf("post-commit projection failure lost creation authority: status=%d participants=%+v body=%s",
			failedProjectionResponse.Code, simulation.allParticipants, failedProjectionResponse.Body.String())
	}
	server.ollamaModelAuthority = nil
	projectionRequest := httptest.NewRequest(
		http.MethodGet,
		"/v1/ui/chat/roleplay?channel_id=story-http&composer_persona_character_id="+createdID,
		nil,
	)
	projectionResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(projectionResponse, projectionRequest)
	var component roleplaySimulationComponentResponse
	if err := json.Unmarshal(projectionResponse.Body.Bytes(), &component); err != nil {
		t.Fatal(err)
	}
	selectedOption := `value="` + createdID + `" data-persona-kind="character" selected`
	if projectionResponse.Code != http.StatusOK ||
		!strings.Contains(component.HTML.Bundle, selectedOption) {
		t.Fatalf("committed identity projection status=%d body=%s",
			projectionResponse.Code, projectionResponse.Body.String())
	}
}

func TestRoleplayDefinitionEndpointsConsumeExactTypedRequests(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	character := roleplay.SimulationCharacterSummary{
		ID: roleplayHTTPViewpoint, WorldID: simulation.world.ID,
		Name: "Definition Keeper", CreatedAt: time.Now().UTC(),
	}
	simulation.worldCharacters = []roleplay.SimulationCharacterSummary{character}
	simulation.names[character.ID] = character.Name
	server := newRoleplayHTTPTestServer(t, simulation)
	tests := []struct {
		path  string
		body  string
		check func() bool
	}{
		{
			"/v1/channels/story-http/roleplay/meters",
			`{"key":"signal","name":"Signal","minimum":-4,"maximum":12,"initial_value":3}`,
			func() bool {
				return simulation.lastMeterDefinition.WorldID == simulation.world.ID &&
					simulation.lastMeterDefinition.Key == "signal" && simulation.lastMeterDefinition.Minimum == -4
			},
		},
		{
			"/v1/channels/story-http/roleplay/interactions",
			`{"key":"calibrate","name":"Calibrate","description":"Adjust a signal.","argument_mode":"required","effects":[{"meter_key":"signal","delta":2}]}`,
			func() bool {
				return strings.HasPrefix(simulation.lastInteraction.ID, "rpa_") &&
					len(simulation.lastInteraction.Effects) == 1 && simulation.lastInteraction.Effects[0].Delta == 2
			},
		},
		{
			"/v1/channels/story-http/roleplay/items",
			`{"name":"Cipher Lens","description":"Reads a signal.","use_policy":"finite","initial_uses":2,"trigger":null,"priority":4,"effects":[{"meter_key":"signal","delta":-1}]}`,
			func() bool {
				return strings.HasPrefix(simulation.lastItem.ID, "rpi_") &&
					simulation.lastItem.UsePolicy == roleplay.ItemUseFinite && simulation.lastItem.InitialUses == 2
			},
		},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewBufferString(test.body))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusCreated || !test.check() {
			t.Errorf("path=%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestRoleplayRevisionedWritesMapConflictsAndPreserveAuthority(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	simulation.names[roleplayHTTPActive] = "Signal Keeper"
	simulation.writePersonaErr = roleplay.ErrSimulationStaleRevision
	server := newRoleplayHTTPTestServer(t, simulation)
	body := `{"expected_revision":7,"summary":"Maps signals","voice":"Precise","traits":[],"goals":[]}`
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/channels/story-http/roleplay/personas/"+roleplayHTTPActive,
		bytes.NewBufferString(body),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusConflict || simulation.lastPersona.ExpectedRevision != 7 ||
		simulation.lastPersona.CharacterID != roleplayHTTPActive || simulation.personaConfigured[roleplayHTTPActive] {
		t.Fatalf("status=%d persona=%+v body=%s", response.Code, simulation.lastPersona, response.Body.String())
	}
}

func TestRoleplayPersonaWriteRequiresExactSelectedWorldCharacter(t *testing.T) {
	t.Parallel()
	foreignCharacterID := "rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	foreignWorldID := "rpw_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	simulation.names[foreignCharacterID] = "Foreign Archivist"
	simulation.characterWorldIDs[foreignCharacterID] = foreignWorldID
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/channels/story-http/roleplay/personas/"+foreignCharacterID,
		bytes.NewBufferString(`{"expected_revision":0,"summary":"Must not cross worlds.","voice":"","traits":[],"goals":[]}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if simulation.lastPersona.CharacterID != "" || simulation.personaConfigured[foreignCharacterID] {
		t.Fatalf("foreign character mutated persona authority: %+v", simulation.lastPersona)
	}
}

func TestRoleplayPersonaWriteAcceptsExactSelectedWorldCharacter(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	character := roleplay.SimulationCharacterSummary{
		ID: roleplayHTTPActive, WorldID: simulation.world.ID, Name: "Signal Keeper", CreatedAt: time.Now().UTC(),
	}
	simulation.characters.Items = []roleplay.SimulationCharacterSummary{character}
	simulation.worldCharacters = []roleplay.SimulationCharacterSummary{character}
	simulation.names[roleplayHTTPActive] = character.Name
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/channels/story-http/roleplay/personas/"+roleplayHTTPActive,
		bytes.NewBufferString(`{"expected_revision":2,"summary":"Keeps the signal map.","voice":"Precise","traits":["Patient"],"goals":["Preserve the archive"]}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if simulation.lastPersona.CharacterID != roleplayHTTPActive || simulation.lastPersona.ExpectedRevision != 2 ||
		simulation.lastPersona.Sheet.Summary != "Keeps the signal map." {
		t.Fatalf("same-world persona was not written exactly: %+v", simulation.lastPersona)
	}
}

func TestRoleplayMeterWriteCarriesActiveCharacterAndRevision(t *testing.T) {
	t.Parallel()
	simulation := configuredRoleplayHTTPTestStore()
	simulation.participants.Items[0].TurnPosition = 0
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/channels/story-http/roleplay/meters/"+roleplayHTTPActive+"/signal",
		bytes.NewBufferString(`{"expected_revision":3,"value":8}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || simulation.lastMeterUpdate.CharacterID != roleplayHTTPActive ||
		simulation.lastMeterUpdate.ExpectedRevision != 3 || simulation.lastMeterUpdate.Value != 8 {
		t.Fatalf("status=%d update=%+v body=%s", response.Code, simulation.lastMeterUpdate, response.Body.String())
	}
}

func TestRoleplayResearchAccessAcceptsExactCharacterBeyondFirstPage(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
	target := roleplay.SimulationCharacterSummary{
		ID: roleplayHTTPActive, WorldID: simulation.world.ID, Name: "Off-page Archivist",
	}
	simulation.characters = roleplay.SimulationCharacterPage{HasMore: true, Items: []roleplay.SimulationCharacterSummary{
		{ID: "rpc_22222222222222222222222222222222", WorldID: simulation.world.ID, Name: "Second"},
		{ID: "rpc_33333333333333333333333333333333", WorldID: simulation.world.ID, Name: "Third"},
		{ID: "rpc_44444444444444444444444444444444", WorldID: simulation.world.ID, Name: "Fourth"},
		{ID: "rpc_55555555555555555555555555555555", WorldID: simulation.world.ID, Name: "Fifth"},
	}}
	simulation.worldCharacters = append(append(
		[]roleplay.SimulationCharacterSummary(nil), simulation.characters.Items...), target,
	)
	simulation.names[target.ID] = target.Name
	simulation.personaConfigured[target.ID] = true
	for _, character := range simulation.characters.Items {
		simulation.names[character.ID] = character.Name
		simulation.personaConfigured[character.ID] = true
	}
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/channels/story-http/roleplay/capabilities/"+target.ID+"/web-research",
		bytes.NewBufferString(`{"enabled":true}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || simulation.capabilityConfigureCalls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, simulation.capabilityConfigureCalls, response.Body.String())
	}
	if !simulation.lastCapability.WebResearch || simulation.lastCapability.CharacterID != target.ID ||
		!strings.Contains(response.Body.String(), "openRoleplayCharacterEditor") ||
		!strings.Contains(response.Body.String(), target.ID) {
		t.Fatalf("off-page exact character was not server-reconciled: %+v body=%s", simulation.lastCapability, response.Body.String())
	}
}

func TestRoleplayResearchAccessRejectsForeignCharacter(t *testing.T) {
	t.Parallel()
	simulation := configuredRoleplayHTTPTestStore()
	foreignCharacterID := "rpc_99999999999999999999999999999999"
	simulation.names[foreignCharacterID] = "Foreign Keeper"
	simulation.characterWorldIDs[foreignCharacterID] = "rpw_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/channels/story-http/roleplay/capabilities/"+foreignCharacterID+"/web-research",
		bytes.NewBufferString(`{"enabled":true}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || simulation.capabilityConfigureCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, simulation.capabilityConfigureCalls, response.Body.String())
	}
}

func TestRoleplayResearchAccessRejectsRemovedPageOffset(t *testing.T) {
	t.Parallel()
	simulation := configuredRoleplayHTTPTestStore()
	server := newRoleplayHTTPTestServer(t, simulation)
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/channels/story-http/roleplay/capabilities/"+roleplayHTTPActive+"/web-research",
		bytes.NewBufferString(`{"enabled":true,"characters_offset":0}`),
	)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || simulation.capabilityConfigureCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, simulation.capabilityConfigureCalls, response.Body.String())
	}
}
