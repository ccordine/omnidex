package roleplay

import (
	"strings"
	"testing"
	"time"
)

const (
	testWorldID     = "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testCharacterID = "rpc_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testSceneID     = "rps_cccccccccccccccccccccccccccccccc"
	testActionID    = "rpt_dddddddddddddddddddddddddddddddd"
)

func TestParseSimulationActionPreservesExactQuotedArgument(t *testing.T) {
	action, err := ParseSimulationAction(`/offer "a careful, quiet promise"`)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != SimulationActionInteraction || action.CommandKey != "offer" ||
		!action.HasArgument || action.Argument != "a careful, quiet promise" {
		t.Fatalf("action=%+v", action)
	}
	for _, malformed := range []string{
		`/offer a promise`, `/offer ""`, `/offer " leading"`, `/offer "escaped\value"`,
		"/offer \"two\" words", " /offer", "/offer\n\"value\"", `/give`,
	} {
		if _, err := ParseSimulationAction(malformed); err == nil {
			t.Fatalf("ParseSimulationAction(%q) unexpectedly succeeded", malformed)
		}
	}
}

func TestCanonicalItemActionRoundTripsEveryAcceptedNameFixture(t *testing.T) {
	names := []string{
		"Field kit",
		"Traveler's Ω kit [Mk II]",
		"signal\trelay — blue",
	}
	for _, name := range names {
		definition := ItemTemplateDefinition{
			ID: "rpi_22222222222222222222222222222222", WorldID: testWorldID,
			Name: name, Description: "A bounded fixture.", UsePolicy: ItemUseInfinite,
			Effects: []MeterDelta{{MeterKey: "signal", Delta: 1}},
		}
		if err := validateItemDefinition(definition); err != nil {
			t.Fatalf("accepted item name %q: %v", name, err)
		}
		for _, kind := range []SimulationActionKind{SimulationActionGive, SimulationActionTake} {
			exact, err := CanonicalItemAction(kind, name)
			if err != nil {
				t.Fatalf("canonical %s %q: %v", kind, name, err)
			}
			parsed, err := ParseSimulationAction(exact)
			if err != nil || parsed.Kind != kind || parsed.Argument != name || !parsed.HasArgument {
				t.Fatalf("round trip %q => %+v error=%v", exact, parsed, err)
			}
		}
	}
	for _, name := range []string{`quoted "item"`, `back\\slash`, "line\nbreak", "carriage\rreturn"} {
		definition := ItemTemplateDefinition{
			ID: "rpi_22222222222222222222222222222222", WorldID: testWorldID,
			Name: name, Description: "A bounded fixture.", UsePolicy: ItemUseInfinite,
			Effects: []MeterDelta{{MeterKey: "signal", Delta: 1}},
		}
		if err := validateItemDefinition(definition); err == nil {
			t.Fatalf("unaddressable item name %q was accepted", name)
		}
	}
}

func TestSimulationDefinitionsSupportUnrelatedGenericConfigurations(t *testing.T) {
	fixtures := []struct {
		meter   MeterDefinition
		command InteractionCommandDefinition
		item    ItemTemplateDefinition
	}{
		{
			meter: MeterDefinition{WorldID: testWorldID, Key: "warmth", Name: "Warmth", Minimum: -10, Maximum: 10},
			command: InteractionCommandDefinition{ID: "rpa_11111111111111111111111111111111", WorldID: testWorldID,
				Key: "reassure", Name: "Reassure", Description: "A steady reassurance is offered.",
				ArgumentMode: CommandArgumentRequired, Effects: []MeterDelta{{MeterKey: "warmth", Delta: 3}}},
			item: ItemTemplateDefinition{ID: "rpi_22222222222222222222222222222222", WorldID: testWorldID,
				Name: "Wool blanket", Description: "A thick woven blanket.", UsePolicy: ItemUseFinite, InitialUses: 2,
				Trigger: &ItemTrigger{MeterKey: "warmth", Direction: ThresholdAtOrBelow, Threshold: -3}, Priority: 8,
				Effects: []MeterDelta{{MeterKey: "warmth", Delta: 6}}},
		},
		{
			meter: MeterDefinition{WorldID: testWorldID, Key: "signal", Name: "Signal", Minimum: 0, Maximum: 100, InitialValue: 50},
			command: InteractionCommandDefinition{ID: "rpa_33333333333333333333333333333333", WorldID: testWorldID,
				Key: "calibrate", Name: "Calibrate", Description: "The receiver is carefully calibrated.",
				ArgumentMode: CommandArgumentNone, Effects: []MeterDelta{{MeterKey: "signal", Delta: 5}}},
			item: ItemTemplateDefinition{ID: "rpi_44444444444444444444444444444444", WorldID: testWorldID,
				Name: "Reference crystal", Description: "A stable frequency reference.", UsePolicy: ItemUseInfinite,
				Priority: -2, Effects: []MeterDelta{{MeterKey: "signal", Delta: 2}}},
		},
	}
	for index, fixture := range fixtures {
		if err := validateMeterDefinition(fixture.meter); err != nil {
			t.Fatalf("fixture %d meter: %v", index, err)
		}
		if err := validateCommandDefinition(fixture.command); err != nil {
			t.Fatalf("fixture %d command: %v", index, err)
		}
		if err := validateItemDefinition(fixture.item); err != nil {
			t.Fatalf("fixture %d item: %v", index, err)
		}
	}
}

func TestSimulationReservedInteractionAndInertItemRules(t *testing.T) {
	reserved := InteractionCommandDefinition{
		ID: "rpa_55555555555555555555555555555555", WorldID: testWorldID,
		Key: "research", Name: "Research", Description: "Reserved external work.",
		ArgumentMode: CommandArgumentRequired, Effects: []MeterDelta{{MeterKey: "signal", Delta: 1}},
	}
	if err := validateCommandDefinition(reserved); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved validation error=%v", err)
	}
	eligible, err := simulationItemEligibleTx(t.Context(), nil, testWorldID, testCharacterID, simulationInventoryCandidate{})
	if err != nil || eligible {
		t.Fatalf("inert item eligible=%v error=%v", eligible, err)
	}
	if got := clampMeterValue(9, 50, 0, 10); got != 10 {
		t.Fatalf("upper clamp=%d", got)
	}
	if got := clampMeterValue(1, -50, 0, 10); got != 0 {
		t.Fatalf("lower clamp=%d", got)
	}
}

func TestSimulationTurnAuthorityValidation(t *testing.T) {
	projection := NarrativeSimulationProjection{
		Schema: NarrativeSimulationProjectionSchemaV1,
		Scene: NarrativeScene{
			Title: "Crossroads", Description: "A quiet road.", ActiveCharacterName: "Ari",
			Initiative: initialSimulationInitiativeClock(),
		},
		Participants: []string{"Ari", "Bex"},
		Viewpoint:    NarrativePersona{Name: "Ari", Summary: "A traveler.", Voice: "Measured.", Traits: []string{}, Goals: []string{}},
		Meters:       []NarrativeMeter{}, Inventory: []NarrativeInventoryItem{},
		VisibleFacts: []string{}, Memories: []string{}, RecentEvents: []string{},
	}
	narrativeAuthority := SimulationNarrativeAuthority{
		WorldID: testWorldID, SceneID: testSceneID, SceneRevision: 3,
		ViewpointID: testCharacterID, ParticipantIDs: []string{testCharacterID, "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
		MeterKeys: []string{}, InventoryItemIDs: []string{}, CanonEventIDs: []string{},
		MemoryIDs: []string{}, TransitionIDs: []string{},
	}
	fingerprint, err := simulationNarrativeDigest(projection, narrativeAuthority)
	if err != nil {
		t.Fatal(err)
	}
	narrativeAuthority.Fingerprint = fingerprint
	authority := SimulationTurnAuthority{
		PreparationID: testActionID, ChannelID: "roleplay-channel", UserMessageID: 7,
		WorldID: testWorldID, SceneID: testSceneID, BaseSceneRevision: 3, SceneRevision: 3,
		ActiveCharacterID: testCharacterID, InputKind: SimulationTurnProse,
		UserTurn: UserTurnAuthority{
			PersonaKind: UserPersonaCharacter, CharacterID: "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
			PersonaName: "Bex", PersonaSummary: "Another traveler.",
			ContributionKind: UserContributionDialogue, ExactText: "Hello.",
		},
		ParticipantCharacterIDs: []string{testCharacterID, "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}, NarrativeProjection: projection,
		GenerationConfig: CharacterGenerationConfig{
			Schema:             CharacterGenerationConfigSchemaV2,
			LibraryCharacterID: "rpl_0123456789abcdef0123456789abcdef", Revision: 1,
		},
		NarrativeAuthority: narrativeAuthority, NarrativeFingerprint: fingerprint,
		CreatedAt: time.Now().UTC(),
	}
	authority.Responders = []SimulationResponderAuthority{{
		Position: 0, CharacterID: testCharacterID,
		GenerationConfig:    authority.GenerationConfig,
		NarrativeProjection: projection, NarrativeAuthority: narrativeAuthority,
		NarrativeFingerprint: fingerprint,
	}}
	authority.ResponderRoutes = []SimulationResponderRoute{{
		Position: 0, CharacterID: testCharacterID,
		GenerationConfig: authority.GenerationConfig, NarrativeFingerprint: fingerprint,
	}}
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
	actingResponder := authority
	actingResponder.ActiveCharacterID = actingResponder.UserTurn.CharacterID
	if err := actingResponder.Validate(); err != nil {
		t.Fatalf("initiative character was rejected as acting persona: %v", err)
	}
	narratorRound := authority
	narratorRound.UserTurn = UserTurnAuthority{
		PersonaKind: UserPersonaNarrator, PersonaName: NarratorPersonaName,
		ContributionKind: UserContributionDirection, ExactText: "Continue the scene.",
	}
	secondProjection := CloneNarrativeSimulationProjection(projection)
	secondProjection.Viewpoint.Name = "Bex"
	secondAuthority := narrativeAuthority
	secondAuthority.ViewpointID = "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	secondFingerprint, err := simulationNarrativeDigest(secondProjection, secondAuthority)
	if err != nil {
		t.Fatal(err)
	}
	secondAuthority.Fingerprint = secondFingerprint
	narratorRound.Responders = append(narratorRound.Responders, SimulationResponderAuthority{
		Position: 1, CharacterID: secondAuthority.ViewpointID,
		GenerationConfig: narratorRound.GenerationConfig, NarrativeProjection: secondProjection,
		NarrativeAuthority: secondAuthority, NarrativeFingerprint: secondFingerprint,
	})
	narratorRound.ResponderRoutes = append(narratorRound.ResponderRoutes, SimulationResponderRoute{
		Position: 1, CharacterID: secondAuthority.ViewpointID,
		GenerationConfig: narratorRound.GenerationConfig, NarrativeFingerprint: secondFingerprint,
	})
	if err := narratorRound.Validate(); err != nil {
		t.Fatalf("narrator response round was rejected: %v", err)
	}
	narratorRound.Responders[1].NarrativeProjection.Scene.Initiative.Turn++
	narratorRound.Responders[1].NarrativeProjection.Scene.Initiative.FictionalTimeTick++
	changedProjection := narratorRound.Responders[1].NarrativeProjection
	changedAuthority := narratorRound.Responders[1].NarrativeAuthority
	changedFingerprint, err := simulationNarrativeDigest(changedProjection, changedAuthority)
	if err != nil {
		t.Fatal(err)
	}
	narratorRound.Responders[1].NarrativeAuthority.Fingerprint = changedFingerprint
	narratorRound.Responders[1].NarrativeFingerprint = changedFingerprint
	narratorRound.ResponderRoutes[1].NarrativeFingerprint = changedFingerprint
	if err := narratorRound.Validate(); err == nil || !strings.Contains(err.Error(), "frozen response round") {
		t.Fatalf("mixed-clock response round error=%v", err)
	}
	authority.ParticipantCharacterIDs = []string{"rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}
	if err := authority.Validate(); err == nil {
		t.Fatal("authority without active participant unexpectedly validated")
	}
	authority.ParticipantCharacterIDs = []string{testCharacterID, "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}
	authority.InputKind = SimulationTurnAction
	if err := authority.Validate(); err == nil {
		t.Fatal("action authority without transition unexpectedly validated")
	}
}

func TestSimulationTurnAdvanceReplayValidationRejectsParticipantTampering(t *testing.T) {
	result := SimulationTurnAdvanceResult{
		OperationID:   testActionID,
		PreparationID: "rpt_11111111111111111111111111111111",
		WorldID:       testWorldID, SceneID: testSceneID,
		PreviousCharacterID: testCharacterID,
		ActiveCharacterID:   "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		BeforeRevision:      4, AfterRevision: 5,
		BeforeInitiative:        SimulationInitiativeClock{Round: 2, Turn: 4, FictionalTimeTick: 3},
		AfterInitiative:         SimulationInitiativeClock{Round: 2, Turn: 5, FictionalTimeTick: 4},
		ParticipantCharacterIDs: []string{testCharacterID, "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
		NarrativeFingerprint:    strings.Repeat("b", 64), CreatedAt: time.Now().UTC(),
	}
	if err := validateAdvanceReplayResult(result, narratorDirectionTurnForInitiative()); err != nil {
		t.Fatal(err)
	}
	result.AfterInitiative.FictionalTimeTick++
	if err := validateAdvanceReplayResult(result, narratorDirectionTurnForInitiative()); err == nil {
		t.Fatal("non-monotonic fictional time unexpectedly validated")
	}
	result.AfterInitiative.FictionalTimeTick--
	result.ParticipantCharacterIDs[1] = testCharacterID
	if err := validateAdvanceReplayResult(result, narratorDirectionTurnForInitiative()); err == nil {
		t.Fatal("duplicated participant replay unexpectedly validated")
	}
	result.ParticipantCharacterIDs = []string{
		testCharacterID,
		"rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"rpc_ffffffffffffffffffffffffffffffff",
	}
	result.ActiveCharacterID = "rpc_ffffffffffffffffffffffffffffffff"
	if err := validateAdvanceReplayResult(result, narratorDirectionTurnForInitiative()); err == nil ||
		!strings.Contains(err.Error(), "exact next eligible participant") {
		t.Fatalf("skipped-cursor replay error=%v", err)
	}
}

func TestNarrativeProjectionIsBoundedValidatedAndDeepCloned(t *testing.T) {
	projection := NarrativeSimulationProjection{
		Schema: NarrativeSimulationProjectionSchemaV1,
		Scene: NarrativeScene{
			Title: "Crossroads", Description: "Two travelers pause.", ActiveCharacterName: "Ari",
			Initiative: initialSimulationInitiativeClock(),
		},
		Participants: []string{"Ari", "Bex"},
		Viewpoint: NarrativePersona{Name: "Ari", Summary: "A patient traveler.", Voice: "Measured.",
			Traits: []string{"patient"}, Goals: []string{"reach the coast"}},
		Meters:    []NarrativeMeter{{Name: "Resolve", Minimum: 0, Maximum: 10, Value: 4}},
		Inventory: []NarrativeInventoryItem{}, VisibleFacts: []string{}, Memories: []string{}, RecentEvents: []string{},
	}
	if err := projection.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := CloneNarrativeSimulationProjection(projection)
	clone.Participants[0] = "Changed"
	clone.Viewpoint.Traits[0] = "changed"
	if projection.Participants[0] != "Ari" || projection.Viewpoint.Traits[0] != "patient" {
		t.Fatal("clone mutated original narrative content")
	}
	if clone.Inventory == nil || clone.VisibleFacts == nil || clone.Memories == nil || clone.RecentEvents == nil {
		t.Fatal("clone lost canonical empty arrays")
	}
	projection.RecentEvents = nil
	if err := projection.Validate(); err == nil {
		t.Fatal("nil narrative event array unexpectedly validated")
	}
}

func TestNarrativeFingerprintCommitsTheExactInitiativeClock(t *testing.T) {
	projection := NarrativeSimulationProjection{
		Schema: NarrativeSimulationProjectionSchemaV1,
		Scene: NarrativeScene{
			Title: "Crossroads", Description: "Two travelers pause.", ActiveCharacterName: "Ari",
			Initiative: initialSimulationInitiativeClock(),
		},
		Participants: []string{"Ari", "Bex"},
		Viewpoint: NarrativePersona{
			Name: "Ari", Summary: "A patient traveler.", Voice: "Measured.",
			Traits: []string{}, Goals: []string{},
		},
		Meters: []NarrativeMeter{}, Inventory: []NarrativeInventoryItem{},
		VisibleFacts: []string{}, Memories: []string{}, RecentEvents: []string{},
	}
	authority := SimulationNarrativeAuthority{
		WorldID: testWorldID, SceneID: testSceneID, SceneRevision: 3,
		ViewpointID:    testCharacterID,
		ParticipantIDs: []string{testCharacterID, "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
		MeterKeys:      []string{}, InventoryItemIDs: []string{}, CanonEventIDs: []string{},
		MemoryIDs: []string{}, TransitionIDs: []string{},
	}
	before, err := simulationNarrativeDigest(projection, authority)
	if err != nil {
		t.Fatal(err)
	}
	base, err := simulationNarrativeBaseDigest(projection, authority)
	if err != nil {
		t.Fatal(err)
	}
	if expected := simulationSHA([]byte(base + ":1:1:0")); before != expected {
		t.Fatalf("clocked narrative fingerprint=%q want %q", before, expected)
	}
	projection.Scene.Initiative.Turn++
	projection.Scene.Initiative.FictionalTimeTick++
	after, err := simulationNarrativeDigest(projection, authority)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("narrative fingerprint did not commit the changed initiative clock")
	}
}
