package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayModelBoundaryHasNoOperationOrToolSurface(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	patterns := []string{
		"internal/assemblyline/*roleplay*.go",
		"internal/assemblyline/conversation_response.go",
		"internal/worker/*roleplay*.go",
		"internal/worker/objective_conversation_response.go",
	}
	paths := make([]string, 0)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, matches...)
	}
	if len(paths) < 4 {
		t.Fatalf("roleplay model boundary sources=%d want at least 4", len(paths))
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			`json:"tools`, `json:"tool_`, `json:"functions`, `json:"operations`,
			"TOOL_CATALOG", "AVAILABLE_TOOLS", "AVAILABLE_INTERACTIONS",
			"Do not use tools", "do not call tools", "function_call", "tool_choice",
			"use_item(", "set_meter(", "give_item(", "take_item(", "remember(",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s exposes forbidden roleplay model authority %q", relative, forbidden)
			}
		}
	}
}

func TestRoleplaySimulationPackageOwnsNoModelInvocation(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "..", "internal", "roleplay"))
	paths, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("roleplay simulation sources are unavailable")
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"internal/llm", "internal/ollama", "internal/modelconfig",
			"PortableJob", "BuildPrompt", "ResponseSchema", "tool_choice", "function_call",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s couples fictional state to model invocation %q", filepath.Base(path), forbidden)
			}
		}
	}
}

func TestRoleplayNarrativeProjectionCarriesNoSimulationAuthority(t *testing.T) {
	t.Parallel()
	projection := roleplay.NarrativeSimulationProjection{
		Schema: roleplay.NarrativeSimulationProjectionSchemaV1,
		Scene: roleplay.NarrativeScene{
			Title: "Observation deck", Description: "A quiet room above the city.",
			ActiveCharacterName: "Mara",
		},
		Participants: []string{"Mara", "Ivo"},
		Viewpoint: roleplay.NarrativePersona{
			Name: "Mara", Summary: "A careful navigator.", Voice: "Measured.",
			Traits: []string{"observant"}, Goals: []string{"map the storm"},
		},
		Meters: []roleplay.NarrativeMeter{{Name: "Focus", Minimum: 0, Maximum: 10, Value: 7}},
		Inventory: []roleplay.NarrativeInventoryItem{{
			Name: "Field notebook", Description: "Weathered pages.", UseDisplay: "2 remaining",
		}},
		VisibleFacts: []string{"The storm moved east."},
		Memories:     []string{"Ivo kept watch during the last crossing."},
		RecentEvents: []string{"Used the field notebook."},
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{
		`"world_id"`, `"scene_id"`, `"character_id"`, `"viewpoint_id"`,
		`"revision"`, `"fingerprint"`, `"created_at"`, `"updated_at"`,
		`"command"`, `"commands"`, `"exact_action"`, `"operation"`,
		`"trigger"`, `"priority"`, `"effects"`, `"transition_id"`,
		`"meter_key"`, `"inventory_item_id"`,
	} {
		if strings.Contains(encoded, forbidden) {
			t.Errorf("roleplay narrative projection exposes simulation authority %s: %s", forbidden, encoded)
		}
	}
}

func TestRoleplayInvariantIsNormativeAndRejectsWriteOnlyResearchCapability(t *testing.T) {
	t.Parallel()
	path := filepath.Clean(filepath.Join("..", "..", "docs", "ROLEPLAY_SIMULATION.md"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"it has no roleplay tool interface",
		"A model response is not a state transition.",
		"Do not expose a research checkbox until that end-to-end consumer exists",
		"browser renders server state and does not maintain a competing simulation",
	} {
		if !strings.Contains(document, required) {
			t.Errorf("roleplay invariant is missing %q", required)
		}
	}
}
