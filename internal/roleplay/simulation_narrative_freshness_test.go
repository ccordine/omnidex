package roleplay

import (
	"errors"
	"strings"
	"testing"
)

func TestPreparedNarrativeFreshnessNamesTheChangedAuthority(t *testing.T) {
	expectedProjection := NarrativeSimulationProjection{
		Schema:       NarrativeSimulationProjectionSchemaV1,
		Scene:        NarrativeScene{Title: "Archive", Description: "Moonlit shelves.", ActiveCharacterName: "Mara"},
		Participants: []string{"Mara", "Gryph"},
		Viewpoint: NarrativePersona{
			Name: "Mara", Summary: "Archivist.", Voice: "Measured.",
			Traits: []string{"careful"}, Goals: []string{"remember"},
		},
		Meters:       []NarrativeMeter{{Name: "Trust", Minimum: 0, Maximum: 10, Value: 4}},
		Inventory:    []NarrativeInventoryItem{{Name: "Key", Description: "Blue glass.", UseDisplay: "infinite"}},
		VisibleFacts: []string{"The seal opens the north observatory in moonlight."},
		Memories:     []string{"Gryph kept the secret."},
		RecentEvents: []string{"Gryph entered the archive."},
	}
	expectedAuthority := SimulationNarrativeAuthority{
		WorldID: testWorldID, SceneID: testSceneID, SceneRevision: 7,
		ViewpointID:    testCharacterID,
		ParticipantIDs: []string{testCharacterID, "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
		MeterKeys:      []string{"trust"}, InventoryItemIDs: []string{"rpv_11111111111111111111111111111111"},
		CanonEventIDs: []string{"rpe_11111111111111111111111111111111"},
		MemoryIDs:     []string{"rpm_11111111111111111111111111111111"},
		TransitionIDs: []string{testActionID}, Fingerprint: strings.Repeat("a", 64),
	}
	if err := requirePreparedNarrative(
		expectedProjection, expectedAuthority, expectedProjection, expectedAuthority,
	); err != nil {
		t.Fatalf("unchanged narrative was rejected: %v", err)
	}

	tests := []struct {
		name, want string
		mutate     func(*NarrativeSimulationProjection, *SimulationNarrativeAuthority)
	}{
		{"scene", "scene", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Scene.Title = "Vault" }},
		{"cast", "cast", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Participants[1] = "Elian" }},
		{"persona", "responding character", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Viewpoint.Voice = "Wry." }},
		{"meter", "meters", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) { p.Meters[0].Value = 5 }},
		{"inventory", "inventory", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.Inventory[0].UseDisplay = "1 remaining"
		}},
		{"canon", "canon", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.VisibleFacts[0] = "The seal opens at dawn."
		}},
		{"memory", "memories", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.Memories[0] = "Gryph revealed the secret."
		}},
		{"events", "simulation events", func(p *NarrativeSimulationProjection, _ *SimulationNarrativeAuthority) {
			p.RecentEvents[0] = "Gryph left."
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualProjection := CloneNarrativeSimulationProjection(expectedProjection)
			actualAuthority := cloneSimulationNarrativeAuthority(expectedAuthority)
			test.mutate(&actualProjection, &actualAuthority)
			err := requirePreparedNarrative(
				expectedProjection, expectedAuthority, actualProjection, actualAuthority,
			)
			if !errors.Is(err, ErrSimulationStaleRevision) || !strings.Contains(err.Error(), test.want) ||
				!strings.Contains(err.Error(), "restore and retry") {
				t.Fatalf("freshness error=%v, want changed authority %q", err, test.want)
			}
		})
	}
}

func cloneSimulationNarrativeAuthority(value SimulationNarrativeAuthority) SimulationNarrativeAuthority {
	value.ParticipantIDs = append([]string(nil), value.ParticipantIDs...)
	value.MeterKeys = append([]string(nil), value.MeterKeys...)
	value.InventoryItemIDs = append([]string(nil), value.InventoryItemIDs...)
	value.CanonEventIDs = append([]string(nil), value.CanonEventIDs...)
	value.MemoryIDs = append([]string(nil), value.MemoryIDs...)
	value.TransitionIDs = append([]string(nil), value.TransitionIDs...)
	return value
}
