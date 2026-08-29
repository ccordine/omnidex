package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplaySimulationRequestsRequireExplicitNumericAuthority(t *testing.T) {
	t.Parallel()
	minimum, maximum, initial := 0, 12, 4
	for _, request := range []roleplayMeterRequest{
		{Key: "focus", Name: "Focus", Maximum: &maximum, InitialValue: &initial},
		{Key: "focus", Name: "Focus", Minimum: &minimum, InitialValue: &initial},
		{Key: "focus", Name: "Focus", Minimum: &minimum, Maximum: &maximum},
	} {
		if _, err := request.definition("rpw_0123456789abcdef0123456789abcdef"); err == nil {
			t.Fatalf("incomplete meter request was accepted: %+v", request)
		}
	}
	priority, initialUses := 0, 0
	if _, err := (roleplayItemRequest{Priority: &priority, Trigger: &roleplayItemTriggerRequest{
		MeterKey: "focus", Direction: "at_or_above",
	}, InitialUses: &initialUses}).definition("rpi_0123456789abcdef0123456789abcdef", "rpw_0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("item trigger without an explicit threshold was accepted")
	}
	if _, err := (roleplayItemRequest{Priority: &priority, Trigger: &roleplayItemTriggerRequest{
		MeterKey: "focus", Direction: "at_or_above", Threshold: &initialUses,
	}}).definition("rpi_0123456789abcdef0123456789abcdef", "rpw_0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("item without explicit initial uses was accepted")
	}
}

func TestRoleplayItemRequestUsesCanonicalSlashArgumentDomain(t *testing.T) {
	t.Parallel()
	priority, initialUses, delta := 0, 0, 1
	base := roleplayItemRequest{
		Description: "A bounded fixture.", UsePolicy: roleplay.ItemUseInfinite,
		InitialUses: &initialUses, Priority: &priority,
		Effects: []roleplayMeterDeltaRequest{{MeterKey: "signal", Delta: &delta}},
	}
	for _, name := range []string{"Field kit", "Traveler's Ω kit [Mk II]", "signal\trelay"} {
		request := base
		request.Name = name
		definition, err := request.definition(
			"rpi_0123456789abcdef0123456789abcdef",
			"rpw_0123456789abcdef0123456789abcdef",
		)
		if err != nil {
			t.Fatalf("definition %q: %v", name, err)
		}
		if err := validateRoleplayItemDefinition(definition); err != nil {
			t.Fatalf("API rejected addressable item name %q: %v", name, err)
		}
		for _, kind := range []roleplay.SimulationActionKind{
			roleplay.SimulationActionGive, roleplay.SimulationActionTake,
		} {
			exact, err := roleplay.CanonicalItemAction(kind, definition.Name)
			if err != nil {
				t.Fatalf("API item %q has no canonical %s command: %v", name, kind, err)
			}
			parsed, err := roleplay.ParseSimulationAction(exact)
			if err != nil || parsed.Argument != definition.Name || parsed.Kind != kind {
				t.Fatalf("API item command %q parsed as %+v error=%v", exact, parsed, err)
			}
		}
	}
	for _, name := range []string{`quoted "item"`, `back\\slash`, "line\nbreak", "carriage\rreturn"} {
		request := base
		request.Name = name
		definition, err := request.definition(
			"rpi_0123456789abcdef0123456789abcdef",
			"rpw_0123456789abcdef0123456789abcdef",
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateRoleplayItemDefinition(definition); err == nil {
			t.Fatalf("API accepted unaddressable item name %q", name)
		}
	}
}

func TestRoleplaySimulationJSONRejectsUnknownAndDuplicateFields(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"name":"Rin","unknown":true}`,
		`{"name":"Rin","name":"Kai"}`,
	} {
		request := httptest.NewRequest("POST", "/v1/channels/story/roleplay/characters", strings.NewReader(body))
		response := httptest.NewRecorder()
		var decoded roleplayCharacterRequest
		if err := decodeExactRoleplayJSON(response, request, "roleplay character request", &decoded); err == nil {
			t.Fatalf("inexact body was accepted: %s", body)
		}
	}
}

func TestRoleplayResearchCapabilityRequiresOnlyExplicitBoolean(t *testing.T) {
	t.Parallel()
	enabled := false
	if err := validateRoleplayResearchCapabilityRequest(roleplayResearchCapabilityRequest{
		Enabled: &enabled,
	}); err != nil {
		t.Fatalf("explicit disabled capability was rejected: %v", err)
	}
	if err := validateRoleplayResearchCapabilityRequest(roleplayResearchCapabilityRequest{}); err == nil {
		t.Fatal("research capability without an explicit boolean was accepted")
	}
}
