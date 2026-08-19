package api

import (
	"bytes"
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

func TestRoleplayDefinitionEndpointsConsumeExactTypedRequests(t *testing.T) {
	t.Parallel()
	simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
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

func TestRoleplayResearchAccessRequiresVisibleSameWorldCharacter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		character roleplay.SimulationCharacterSummary
		routeID   string
		status    int
		calls     int
	}{
		{
			name: "visible same world",
			character: roleplay.SimulationCharacterSummary{
				ID: roleplayHTTPActive, WorldID: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "Signal Keeper",
			},
			routeID: roleplayHTTPActive, status: http.StatusOK, calls: 1,
		},
		{
			name: "identity is not visible",
			character: roleplay.SimulationCharacterSummary{
				ID: roleplayHTTPActive, WorldID: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "Signal Keeper",
			},
			routeID: "rpc_99999999999999999999999999999999", status: http.StatusConflict, calls: 0,
		},
		{
			name: "visible identity belongs to another world",
			character: roleplay.SimulationCharacterSummary{
				ID: roleplayHTTPActive, WorldID: "rpw_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Name: "Foreign Keeper",
			},
			routeID: roleplayHTTPActive, status: http.StatusConflict, calls: 0,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			simulation := newRoleplaySimulationTestStore(roleplayHTTPChannelID)
			simulation.characters.Items = []roleplay.SimulationCharacterSummary{test.character}
			simulation.names[test.character.ID] = test.character.Name
			simulation.personaConfigured[test.character.ID] = true
			server := newRoleplayHTTPTestServer(t, simulation)
			request := httptest.NewRequest(
				http.MethodPut,
				"/v1/channels/story-http/roleplay/capabilities/"+test.routeID+"/web-research",
				bytes.NewBufferString(`{"enabled":true,"characters_offset":0}`),
			)
			response := httptest.NewRecorder()

			server.Handler().ServeHTTP(response, request)

			if response.Code != test.status || simulation.capabilityConfigureCalls != test.calls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, simulation.capabilityConfigureCalls, response.Body.String())
			}
			if test.calls == 1 && (!simulation.lastCapability.WebResearch ||
				!strings.Contains(response.Body.String(), "/research")) {
				t.Fatalf("configured access was not server-reconciled: %+v body=%s", simulation.lastCapability, response.Body.String())
			}
		})
	}
}
