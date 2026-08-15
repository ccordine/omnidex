package assemblyline

import (
	"strings"
	"testing"
)

func TestTypeScriptCompilerRepairPromptCarriesExactLocalBindings(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name                string
		signature           string
		source              string
		bindings            []TypeScriptRepairBinding
		unavailable         []TypeScriptRepairBinding
		required            []string
		unavailableRequired []string
	}{
		{
			name:      "inventory callback",
			signature: "function UpdateInventory(index: number, actions: InventoryActions): void",
			source:    "  actions.commit(index, missingValue);",
			bindings: []TypeScriptRepairBinding{
				{Name: "actions", Type: "InventoryActions", Members: []string{"commit: (index: number, value: number) => void"}},
				{Name: "index", Type: "number"},
			},
			unavailable: []TypeScriptRepairBinding{
				{Name: "missingValue", Type: "number"},
				{Name: "previousValue", Type: "number"},
			},
			required:            []string{`"name":"actions"`, `"type":"InventoryActions"`, "commit: (index: number, value: number) => void", `"name":"index"`},
			unavailableRequired: []string{`"name":"missingValue"`, `"name":"previousValue"`},
		},
		{
			name:      "schedule callback",
			signature: "function MoveAppointment(day: number, schedule: ScheduleActions): void",
			source:    "  schedule.move(day, nextDay);",
			bindings: []TypeScriptRepairBinding{
				{Name: "day", Type: "number"},
				{Name: "schedule", Type: "ScheduleActions", Members: []string{"move: (from: number, to: number) => void"}},
			},
			unavailable: []TypeScriptRepairBinding{
				{Name: "nextDay", Type: "number"},
				{Name: "priorDay", Type: "number"},
			},
			required:            []string{`"name":"day"`, `"name":"schedule"`, "move: (from: number, to: number) => void"},
			unavailableRequired: []string{`"name":"nextDay"`, `"name":"priorDay"`},
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			region := TypeScriptFragmentRepairRegion{
				Kind: TypeScriptRepairRegionCompilerOwner, StartLine: 7, EndLine: 7,
				Source: fixture.source, Bindings: fixture.bindings,
				UnavailableBindings: fixture.unavailable,
			}
			prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
				Signature: fixture.signature, RepairRegion: &region,
				RequiredChange: "Eliminate the exact compiler failure using only bindings available at the failure.",
				Diagnostic:     "TS2304: one identifier is unavailable at the failing expression.",
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(prompt, "LOCAL_BINDINGS_AVAILABLE_AT_FAILURE_JSON:") ||
				!strings.Contains(prompt, "Identifiers absent from the local bindings") {
				t.Fatalf("prompt omitted lexical authority:\n%s", prompt)
			}
			for _, value := range fixture.required {
				if !strings.Contains(prompt, value) {
					t.Fatalf("prompt omitted %q:\n%s", value, prompt)
				}
			}
			bindingsSection := strings.SplitN(
				strings.SplitN(prompt, "LOCAL_BINDINGS_AVAILABLE_AT_FAILURE_JSON:\n", 2)[1],
				"\n\nNESTED_BINDINGS_UNAVAILABLE_AT_FAILURE_JSON:", 2,
			)[0]
			for _, value := range fixture.unavailableRequired {
				if strings.Contains(bindingsSection, value) {
					t.Fatalf("available bindings included unavailable name %q:\n%s", value, bindingsSection)
				}
				if !strings.Contains(prompt, "NESTED_BINDINGS_UNAVAILABLE_AT_FAILURE_JSON:") ||
					!strings.Contains(prompt, value) {
					t.Fatalf("prompt omitted unavailable binding %q:\n%s", value, prompt)
				}
			}
		})
	}
}

func TestTypeScriptRepairRegionsRequireBindingsOnlyForCompilerAuthority(t *testing.T) {
	t.Parallel()
	compiler := TypeScriptFragmentRepairRegion{
		Kind: TypeScriptRepairRegionCompilerOwner, StartLine: 1, EndLine: 1,
		Source: "return missing;",
	}
	if err := compiler.validate(); err == nil || !strings.Contains(err.Error(), "local bindings") {
		t.Fatalf("compiler region without bindings error=%v", err)
	}
	syntax := TypeScriptFragmentRepairRegion{
		Kind: TypeScriptRepairRegionSyntaxWindow, StartLine: 1, EndLine: 1,
		Source: "return +;", Bindings: []TypeScriptRepairBinding{{Name: "value", Type: "number"}},
	}
	if err := syntax.validate(); err == nil || !strings.Contains(err.Error(), "cannot carry") {
		t.Fatalf("syntax region accepted compiler bindings: %v", err)
	}
}
