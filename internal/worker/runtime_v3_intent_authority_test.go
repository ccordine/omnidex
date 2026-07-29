package worker

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
)

func TestIntentActionShapeValidationDoesNotRewriteModelSemantics(t *testing.T) {
	input := v3IntentInput{
		OperationKind:      v3OperationUserRequest,
		CurrentInstruction: "Glorbnicate this contraption until every wobble is gone.",
	}
	candidate := artifacts.IntentArtifact{
		UserGoal: "Correct every requested wobble",
		Mode:     "execute",
		Objectives: []artifacts.Objective{{
			ID: "correct", Description: "Correct the requested behavior", Priority: 100,
			RequiresAction: true, RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
			AcceptanceCriteria: []string{"The requested observable behavior is present"},
		}},
		Constraints:          []string{"Retain the accepted mechanism"},
		CompletionCriteria:   []string{"The authoritative verification succeeds"},
		RequiresAction:       true,
		RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
		MemoryMode:           artifacts.MemoryModeOff,
	}

	before := candidate
	if err := validateV3IntentActionShape(input, candidate); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(candidate, before) {
		t.Fatalf("shape validation rewrote semantic intent: before=%+v after=%+v", before, candidate)
	}
}

func TestIntentActionShapeRejectsAmbiguousActionObjectiveMapping(t *testing.T) {
	input := v3IntentInput{OperationKind: v3OperationUserRequest, CurrentInstruction: "Perform both requested changes."}
	candidate := artifacts.IntentArtifact{RequiresAction: true, Objectives: []artifacts.Objective{
		{ID: "first", RequiresAction: true},
		{ID: "second", RequiresAction: true},
	}}

	err := validateV3IntentActionShape(input, candidate)
	if err == nil || !strings.Contains(err.Error(), "exactly one action objective") {
		t.Fatalf("validateV3IntentActionShape() err=%v, want explicit mapping failure", err)
	}
}

func TestIntentActionShapeRejectsExtraAdviceObjectiveOnActionTurn(t *testing.T) {
	input := v3IntentInput{OperationKind: v3OperationUserRequest, CurrentInstruction: "Build it and report the result."}
	candidate := artifacts.IntentArtifact{RequiresAction: true, Objectives: []artifacts.Objective{
		{ID: "build", Description: "Build it", Priority: 100, RequiresAction: true},
		{ID: "explain", Description: "Report the result", Priority: 90},
	}}

	err := validateV3IntentActionShape(input, candidate)
	if err == nil || !strings.Contains(err.Error(), "one cohesive objective") {
		t.Fatalf("validateV3IntentActionShape() err=%v, want cohesive-objective failure", err)
	}
}

func TestIntentActionShapeLeavesNonUserTransportAlone(t *testing.T) {
	input := v3IntentInput{OperationKind: v3OperationScrumPlay, CurrentInstruction: "Execute the authoritative Scrum card task."}
	candidate := artifacts.IntentArtifact{UserGoal: "Implement card", Constraints: []string{"Keep tenant boundaries"}}
	if err := validateV3IntentActionShape(input, candidate); err != nil {
		t.Fatal(err)
	}
}

func TestIntentActionShapeConstrainsActionableScrumChannel(t *testing.T) {
	input := v3IntentInput{OperationKind: v3OperationScrumChannel, CurrentInstruction: "Fix it and report back."}
	candidate := artifacts.IntentArtifact{RequiresAction: true, Objectives: []artifacts.Objective{
		{ID: "fix", RequiresAction: true},
		{ID: "report"},
	}}
	if err := validateV3IntentActionShape(input, candidate); err == nil || !strings.Contains(err.Error(), "one cohesive objective") {
		t.Fatalf("actionable Scrum channel hierarchy error=%v", err)
	}
}

func TestIntentSourceHasNoHeuristicRequirementCompiler(t *testing.T) {
	for _, removed := range []string{"runtime_v3_intent_compiler.go", "runtime_v3_intent_coverage.go"} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("removed intent heuristic %s still exists", removed)
		}
	}
	for _, path := range []string{"runtime_v3_support.go", "runtime_v3_repair.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"genericNonAnswer", "specialistRepairModels", "PREVIOUS_INVALID_RESPONSE", "v3SpecialistMechanicalRepairAction"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains semantic or fallback mechanism %q", path, forbidden)
			}
		}
	}
}

func TestIntentInputRejectsRemovedWriteOnlyMetadata(t *testing.T) {
	removed := []string{
		"persistent_execution",
		"planning_passes",
		"review_always",
		"allow_missing_tools",
		"reasoning_level",
		"autonomy_mode",
		"approval_mode",
		"verification_mode",
		"verification_iterations",
		"architect_mode",
		"web_search",
		"workspace_scan",
	}
	for _, key := range removed {
		t.Run(key, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{key: "on"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = (&Service{}).buildV3IntentInput(model.Job{
				Instruction: "Explain the current state.",
				Metadata:    raw,
			})
			if err == nil || !strings.Contains(err.Error(), key+" was removed") {
				t.Fatalf("buildV3IntentInput() error=%v, want explicit removal for %s", err, key)
			}
		})
	}
}
