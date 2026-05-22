package omni

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExtractValidatedPlaybookRequiresAcceptedRun(t *testing.T) {
	result := CommandDecisionResult{
		ExitCode: 0,
		ObjectiveLedger: []StructuredObjective{{
			ID:       "create_notes_crud",
			Status:   "pending",
			Required: true,
			Source:   structuredObjectiveSourceUserExplicit,
		}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "npm run build", ExitCode: 0},
		},
	}
	if _, ok := extractValidatedPlaybook("build a notes app", result, "test"); ok {
		t.Fatal("pending objective should prevent playbook extraction")
	}
}

func TestExtractValidatedPlaybookStoresSuccessfulCommandSequence(t *testing.T) {
	result := CommandDecisionResult{
		ExitCode: 0,
		Elapsed:  2 * time.Second,
		ObjectiveLedger: []StructuredObjective{{
			ID:          "create_notes_crud",
			Description: "Create notes CRUD",
			Status:      "satisfied",
			Evidence:    "npm run build passed",
			Required:    true,
			Source:      structuredObjectiveSourceUserExplicit,
		}},
		Observations: []StructuredCommandObservation{
			{Step: 1, Command: "sed -n '1,120p' package.json", ExitCode: 0},
			{Step: 2, Command: "cat <<'EOF' > src/App.jsx\nnotes\nEOF", ExitCode: 0},
			{Step: 3, Command: "npm run build", ExitCode: 0, Stdout: "built"},
		},
	}
	memory, ok := extractValidatedPlaybook("build a React notes CRUD app", result, "test-provider")
	if !ok {
		t.Fatal("expected playbook")
	}
	if memory.Kind != validatedPlaybookKind {
		t.Fatalf("kind=%q", memory.Kind)
	}
	if !stringListContains(memory.Tags, "validated-playbook") || !stringListContains(memory.Tags, "react") || !stringListContains(memory.Tags, "notes") {
		t.Fatalf("tags=%v", memory.Tags)
	}
	var playbook ValidatedPlaybook
	if err := json.Unmarshal([]byte(memory.Content), &playbook); err != nil {
		t.Fatalf("decode playbook: %v", err)
	}
	if playbook.Confidence < 80 {
		t.Fatalf("confidence=%d", playbook.Confidence)
	}
	if len(playbook.CommandSequence) != 2 {
		t.Fatalf("commands=%v", playbook.CommandSequence)
	}
	if playbook.CommandSequence[0] == "sed -n '1,120p' package.json" {
		t.Fatalf("pure read-only command should not be in command sequence")
	}
	if !strings.Contains(playbook.ScopePolicy, "advisory_only") {
		t.Fatalf("scope policy=%q", playbook.ScopePolicy)
	}
}

func TestValidatedPlaybookMemorySummaryIsCompactAdvisoryContext(t *testing.T) {
	memory := SessionMemory{
		Kind: validatedPlaybookKind,
		Content: `{
			"name": "react_notes",
			"task_pattern": "build a notes app",
			"command_sequence": ["create files", "npm run build"],
			"validation_signals": ["npm run build"],
			"confidence": 90,
			"scope_policy": "advisory_only"
		}`,
	}
	summary := validatedPlaybookMemorySummary(memory)
	for _, want := range []string{"name=react_notes", "confidence=90", "commands=create files -> npm run build", "scope_policy=advisory_only"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q: %s", want, summary)
		}
	}
}
