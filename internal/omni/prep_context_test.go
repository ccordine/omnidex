package omni

import (
	"context"
	"strings"
	"testing"
)

func TestPrepContextBundleHasProvenanceBudgetAndValidation(t *testing.T) {
	workspace := t.TempDir()
	survey := WorksiteSurvey{
		WorkspacePath:    workspace,
		ProjectState:     projectStateExistingGoProject,
		PackageManager:   packageManagerNone,
		AllowedOperation: true,
		Evidence:         []string{"go.mod exists"},
	}
	route := TaskRoute{
		Intent:               "fix scope drift",
		LikelyFiles:          []string{"internal/omni/llm_command.go"},
		VerificationCommands: []string{"go test ./internal/omni"},
		Reasons:              []string{"llm_command.go owns structured command runtime"},
		Confidence:           85,
	}
	bundle := NewPrepContextBundle("task-1", workspace, survey, ContextToolPlan{NeedsShell: true, Tools: []string{"shell"}}, route, []SessionMemory{
		{Kind: "documentation_brief", Content: "Use the Go testing package conventions.", Tags: []string{"documentation"}, CreatedAt: "2026-05-21T10:00:00Z"},
	})

	if bundle.ContextBudgetUsed == 0 || bundle.ContextBudgetLimit == 0 {
		t.Fatalf("budget not recorded: %#v", bundle)
	}
	if len(bundle.Evidence) == 0 {
		t.Fatalf("expected prep evidence: %#v", bundle)
	}
	for _, brief := range allPrepBriefs(bundle) {
		if len(brief.EvidenceIDs) == 0 || len(brief.UsedBy) == 0 {
			t.Fatalf("brief missing routing/provenance: %#v", brief)
		}
	}
	validation := ValidatePrepContextBundle(bundle, ContextToolPlan{NeedsShell: true, Tools: []string{"shell"}})
	if !validation.Valid {
		t.Fatalf("validation = %#v", validation)
	}
}

func TestPrepContextValidationRejectsMissingProvenanceAndOversize(t *testing.T) {
	bundle := PrepContextBundle{
		WorkspacePath:      "/tmp/workspace",
		WorksiteSurvey:     WorksiteSurvey{WorkspacePath: "/tmp/workspace", ProjectState: projectStateExistingProject},
		MemoryChecked:      true,
		ContextBudgetUsed:  200,
		ContextBudgetLimit: 100,
		MemoryBriefs: []PrepBrief{{
			ID:      "brief",
			Kind:    "memory",
			Content: "missing evidence ids and used_by",
		}},
	}
	validation := ValidatePrepContextBundle(bundle, DefaultContextToolPlan())
	if validation.Valid {
		t.Fatalf("validation unexpectedly valid: %#v", validation)
	}
	if !containsCodebaseString(validation.Failures, "prep_context_too_large") {
		t.Fatalf("missing prep_context_too_large: %#v", validation)
	}
	if !containsCodebaseString(validation.Failures, "prep_brief_missing_provenance") {
		t.Fatalf("missing prep_brief_missing_provenance: %#v", validation)
	}
}

func TestPrepareInteractiveTurnContextEmitsBundleValidationEvents(t *testing.T) {
	workspace := t.TempDir()
	writeCodebaseTestFile(t, workspace, "go.mod", "module example.com/app\n")
	writeCodebaseTestFile(t, workspace, "main.go", "package main\nfunc main() {}\n")
	app := NewApp(strings.NewReader(""), &strings.Builder{}, &strings.Builder{})
	events := []Event{}

	prep := app.prepareInteractiveTurnContext(context.Background(), "fix the command loop", workspace, func(eventType, summary string, details map[string]string) {
		events = append(events, Event{Type: eventType, Summary: summary, Details: details})
	})

	if len(prep.Bundle.Evidence) == 0 {
		t.Fatalf("expected bundle evidence: %#v", prep.Bundle)
	}
	if !prep.Validation.Valid {
		t.Fatalf("prep validation failed: %#v", prep.Validation)
	}
	for _, want := range []string{"prep_started", "prep_workspace_scan_completed", "prep_context_built", "prep_context_validated", "prep_completed"} {
		if countEventsOfType(events, want) == 0 {
			t.Fatalf("missing %s in events: %#v", want, events)
		}
	}
}
