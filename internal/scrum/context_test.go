package scrum

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnprovenCardTicketIsDisplayOnly(t *testing.T) {
	t.Parallel()
	lines, err := ContextLinesFromMetadata(json.RawMessage(`{
		"source":"omni-scrum","project_id":1,"scrum_card_id":"card-1",
		"scrum_card_title":"Card","scrum_card_description":"Authoritative description",
		"scrum_checklist":"","scrum_test_criteria":"","scrum_return_column":"",
		"scrum_channel_origin":false,"scrum_channel_operation_id":"","model_config":{},
		"telemetry_run_id":"00000000-0000-4000-8000-000000000001"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Authoritative description") {
		t.Fatalf("description missing from context: %q", joined)
	}
	if strings.Contains(joined, "legacy unproven ticket") || strings.Contains(joined, "Card ticket") {
		t.Fatalf("display-only ticket entered execution context: %q", joined)
	}
	if strings.Contains(joined, "legacy-ai-tag") || strings.Contains(joined, "Tags:") {
		t.Fatalf("unproven tag entered execution context: %q", joined)
	}
	if strings.Contains(joined, "card-1") || strings.Contains(joined, "Card ID:") {
		t.Fatalf("code-owned card identity entered execution context: %q", joined)
	}
}

func TestCardContextHasNoTicketAuthorityField(t *testing.T) {
	t.Parallel()
	lines, err := AppendCardContextLines(nil, CardContext{Description: "Exact description"})
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(lines, "\n"); joined != "Description:\nExact description" {
		t.Fatalf("context=%q", joined)
	}
}

func TestRepositoryPlacementAndRetiredRecipeStayOutOfModelContext(t *testing.T) {
	t.Parallel()
	for _, forbidden := range []string{
		"/private/workspace", "internal/secret/path.go", "retired-recipe",
		"Project directory:", "Reference files:", "Recipe ID:",
	} {
		raw := json.RawMessage(`{
			"source":"omni-scrum","project_id":1,"scrum_card_id":"card-1",
			"scrum_card_title":"Bounded semantic objective",
			"scrum_card_description":"Preserve the declared behavior",
			"scrum_checklist":"","scrum_test_criteria":"","scrum_return_column":"",
			"scrum_channel_origin":false,"scrum_channel_operation_id":"","model_config":{},
			"telemetry_run_id":"00000000-0000-4000-8000-000000000001",
			"forbidden":"` + forbidden + `"
		}`)
		if _, err := ContextLinesFromMetadata(raw); err == nil {
			t.Fatalf("repository placement or retired recipe field %q was accepted", forbidden)
		}
	}
}

func TestStoredJobMetadataRequiresExactCodeOwnedTelemetryBinding(t *testing.T) {
	t.Parallel()
	base := `{"source":"omni-scrum","project_id":1,"scrum_card_id":"card-1","scrum_card_title":"Card","scrum_card_description":"","scrum_checklist":"","scrum_test_criteria":"","scrum_return_column":"","scrum_channel_origin":false,"scrum_channel_operation_id":"","model_config":{}`
	for name, raw := range map[string]string{
		"missing":   base + `}`,
		"uppercase": base + `,"telemetry_run_id":"00000000-0000-4000-8000-00000000000A"}`,
		"unknown":   base + `,"telemetry_run_id":"00000000-0000-4000-8000-000000000001","extra":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeStoredJobMetadata(json.RawMessage(raw)); err == nil {
				t.Fatal("inexact stored Scrum metadata was accepted")
			}
		})
	}
	valid := base + `,"telemetry_run_id":"00000000-0000-4000-8000-000000000001"}`
	if _, err := DecodeStoredJobMetadata(json.RawMessage(valid)); err != nil {
		t.Fatalf("exact stored Scrum metadata rejected: %v", err)
	}
	if _, err := DecodeJobMetadata(json.RawMessage(valid)); err == nil {
		t.Fatal("enqueue Scrum metadata accepted caller-selected telemetry binding")
	}
}

func TestFormatChecklistRejectsInvalidDurableItems(t *testing.T) {
	t.Parallel()
	for name, items := range map[string][]ChecklistItem{
		"blank ID":   {{Text: "work"}},
		"blank text": {{ID: "item-1", Text: "  "}},
		"duplicate":  {{ID: "item-1", Text: "one"}, {ID: "item-1", Text: "two"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := FormatChecklist(items); err == nil {
				t.Fatal("invalid durable checklist was silently projected")
			}
		})
	}
}
