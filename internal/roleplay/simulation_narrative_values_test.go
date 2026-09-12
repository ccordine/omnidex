package roleplay

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPreparedNarrativeComparesEveryProjectedValueAndSource(t *testing.T) {
	before := NarrativeSimulationProjection{
		Schema: NarrativeSimulationProjectionSchemaV1,
		Scene: NarrativeScene{Title: "Observatory", Description: "An open room.", ActiveCharacterName: "Observer",
			Initiative: SimulationInitiativeClock{Round: 1, Turn: 1, FictionalTimeTick: 1}},
		Participants: []string{"Observer"}, Viewpoint: NarrativePersona{Name: "Observer", Summary: "An observer.", Voice: "Clear."},
		Meters:       []NarrativeMeter{{Name: "Heat", Minimum: 0, Maximum: 10, Value: 3}},
		Inventory:    []NarrativeInventoryItem{{Name: "Instrument", Description: "An instrument.", UseDisplay: "Infinite"}},
		VisibleFacts: []string{"The window is open."}, Memories: []string{"The previous visit was quiet."},
		RecentEvents: []string{"The instrument was placed on the table."},
	}
	source := SimulationNarrativeAuthority{
		WorldID: "world", SceneID: "scene", SceneRevision: 1, ViewpointID: "observer", ParticipantIDs: []string{"observer"},
		MeterKeys: []string{"heat"}, InventoryItemIDs: []string{"instrument"}, CanonEventIDs: []string{"fact"},
		MemoryIDs: []string{"memory"}, TransitionIDs: []string{"transition"},
	}
	for _, change := range []struct {
		name, category string
		apply          func(*NarrativeSimulationProjection, *SimulationNarrativeAuthority)
	}{
		{"schema", "projection schema", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Schema = "other" }},
		{"world", "scene", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) { a.WorldID = "other" }},
		{"scene identity", "scene", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) { a.SceneID = "other" }},
		{"scene revision", "scene", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) { a.SceneRevision++ }},
		{"scene title", "scene", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Scene.Title = "Other room" }},
		{"initiative", "scene", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Scene.Initiative.Turn++ }},
		{"cast names", "cast", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Participants[0] = "Other" }},
		{"cast identities", "cast", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) {
			a.ParticipantIDs = []string{"other"}
		}},
		{"viewpoint identity", "responding character", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) { a.ViewpointID = "other" }},
		{"viewpoint content", "responding character", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Viewpoint.Voice = "Quiet." }},
		{"ongoing action", "ongoing actions", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.OngoingActions = []NarrativeOngoingAction{{CharacterName: "Observer", Action: "Reading."}}
		}},
		{"action source", "ongoing actions", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) {
			a.OngoingActionStateIDs = []string{"state"}
		}},
		{"action character", "ongoing actions", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) {
			a.OngoingActionCharacterIDs = []string{"observer"}
		}},
		{"meter value", "meters", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Meters[0].Value++ }},
		{"meter identity", "meters", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) {
			a.MeterKeys = []string{"other"}
		}},
		{"inventory content", "inventory", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.Inventory[0].UseDisplay = "1"
		}},
		{"inventory identity", "inventory", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) {
			a.InventoryItemIDs = []string{"other"}
		}},
		{"canon content", "canon", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.VisibleFacts[0] = "The window is closed."
		}},
		{"canon source", "canon", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) {
			a.CanonEventIDs = []string{"other"}
		}},
		{"memory content", "memories", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.Memories[0] = "The last visit was loud."
		}},
		{"memory source", "memories", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) {
			a.MemoryIDs = []string{"other"}
		}},
		{"event content", "simulation events", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.RecentEvents[0] = "The instrument was removed."
		}},
		{"event source", "simulation events", func(_ *NarrativeSimulationProjection, a *SimulationNarrativeAuthority) {
			a.TransitionIDs = []string{"other"}
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			after := CloneNarrativeSimulationProjection(before)
			afterSource := source
			change.apply(&after, &afterSource)
			err := requirePreparedNarrative(before, source, after, afterSource)
			if !errors.Is(err, ErrSimulationStaleRevision) || !strings.Contains(err.Error(), change.category) {
				t.Fatalf("changed %s was not identified: %v", change.name, err)
			}
		})
	}
	if err := requirePreparedNarrative(before, source, CloneNarrativeSimulationProjection(before), source); err != nil {
		t.Fatalf("equal values failed without a fingerprint: %v", err)
	}
	encoded, err := json.Marshal(struct {
		Projection NarrativeSimulationProjection
		Source     SimulationNarrativeAuthority
	}{before, source})
	if err != nil || strings.Contains(string(encoded), "fingerprint") {
		t.Fatalf("projection retained a fingerprint: %s / %v", encoded, err)
	}
}

func TestEmptyNarrativeSourceCollectionsDoNotCreateAChange(t *testing.T) {
	projection := NarrativeSimulationProjection{Schema: NarrativeSimulationProjectionSchemaV1}
	source := SimulationNarrativeAuthority{}
	emptySource := SimulationNarrativeAuthority{
		ParticipantIDs: []string{}, OngoingActionStateIDs: []string{}, OngoingActionCharacterIDs: []string{},
		MeterKeys: []string{}, InventoryItemIDs: []string{}, CanonEventIDs: []string{}, MemoryIDs: []string{}, TransitionIDs: []string{},
	}
	if err := requirePreparedNarrative(projection, source, projection, emptySource); err != nil {
		t.Fatalf("empty source lists created a false state change: %v", err)
	}
}

func TestStoredNarrativeRejectsRetiredFingerprintFields(t *testing.T) {
	for _, raw := range []string{
		`{"narrative_fingerprint":"obsolete"}`,
		`{"narrative_authority":{"fingerprint":"obsolete"}}`,
		`{"responders":[{"narrative_fingerprint":"obsolete"}]}`,
		`{"responder_routes":[{"narrative_fingerprint":"obsolete"}]}`,
	} {
		if _, err := decodeTurnAuthority([]byte(raw)); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("retired fingerprint was not rejected at decode: %s / %v", raw, err)
		}
	}
}
