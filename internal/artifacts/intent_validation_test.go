package artifacts

import (
	"fmt"
	"strings"
	"testing"
)

func TestIntentValidationRejectsTooManyAcceptanceCriteria(t *testing.T) {
	criteria := make([]string, 0, 13)
	for index := 0; index < 13; index++ {
		criteria = append(criteria, fmt.Sprintf("criterion %d", index+1))
	}
	intent := IntentArtifact{
		UserGoal: "Build the app", Mode: "execute", MemoryMode: MemoryModeOff,
		RequiresAction: true,
		Objectives: []Objective{{
			ID: "build", Description: "Build the app", Priority: 100, RequiresAction: true,
			AcceptanceCriteria: criteria,
		}},
		CompletionCriteria: []string{"tests pass"},
	}
	err := intent.Validate()
	if err == nil || !strings.Contains(err.Error(), "at most 12") {
		t.Fatalf("Validate() err=%v, want explicit acceptance-criteria limit", err)
	}
}
