package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
)

func TestIntentGroundingRejectsInventedFileName(t *testing.T) {
	input := v3IntentInput{CurrentInstruction: "Build a small Go task tracker with tests."}
	intent := artifacts.IntentArtifact{Objectives: []artifacts.Objective{{
		ID: "build", Description: "Build the task tracker",
		AcceptanceCriteria: []string{"A file named pocket_tasks.go exists"},
	}}}
	err := validateV3IntentGrounding(input, intent)
	if err == nil || !strings.Contains(err.Error(), "pocket_tasks.go") {
		t.Fatalf("validateV3IntentGrounding() err=%v, want invented path rejection", err)
	}
}

func TestIntentGroundingRequiresLibraryRestrictionAsConstraint(t *testing.T) {
	input := v3IntentInput{CurrentInstruction: "Use only the Go standard library."}
	intent := artifacts.IntentArtifact{Objectives: []artifacts.Objective{{
		ID: "build", Description: "Build the app",
		AcceptanceCriteria: []string{"Only the Go standard library is used"},
	}}}
	err := validateV3IntentGrounding(input, intent)
	if err == nil || !strings.Contains(err.Error(), "must be in constraints") ||
		!strings.Contains(err.Error(), "objectives[0].acceptance_criteria[0]") ||
		!strings.Contains(err.Error(), "delete this exact array element") {
		t.Fatalf("validateV3IntentGrounding() err=%v, want constraint classification rejection", err)
	}

	intent.Objectives[0].AcceptanceCriteria = []string{"The app works"}
	intent.Constraints = []string{"Use only the Go standard library"}
	if err := validateV3IntentGrounding(input, intent); err != nil {
		t.Fatalf("properly classified constraint rejected: %v", err)
	}
}

func TestIntentGroundingRequiresSeparationRequirementAsConstraint(t *testing.T) {
	input := v3IntentInput{CurrentInstruction: "Separate domain/storage logic from command parsing."}
	intent := artifacts.IntentArtifact{Objectives: []artifacts.Objective{{
		ID: "build", Description: "Build the app",
		AcceptanceCriteria: []string{"Domain/storage logic is separated from command parsing."},
	}}}
	err := validateV3IntentGrounding(input, intent)
	if err == nil || !strings.Contains(err.Error(), "must be in constraints") {
		t.Fatalf("validateV3IntentGrounding() err=%v, want separation constraint rejection", err)
	}

	intent.Objectives[0].AcceptanceCriteria = []string{"The app works"}
	intent.Constraints = []string{"Separate domain/storage logic from command parsing"}
	if err := validateV3IntentGrounding(input, intent); err != nil {
		t.Fatalf("properly classified separation constraint rejected: %v", err)
	}
}
