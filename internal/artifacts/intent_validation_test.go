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

func TestIntentValidationRejectsUnboundedOrInexactLedgerInputs(t *testing.T) {
	base := IntentArtifact{
		UserGoal: "Build the app", Mode: "execute", MemoryMode: MemoryModeOff,
		RequiresAction: true,
		Objectives: []Objective{{
			ID: "build", Description: "Build the app", Priority: 100, RequiresAction: true,
			AcceptanceCriteria: []string{"tests pass"},
		}},
		CompletionCriteria: []string{"tests pass"},
	}
	cases := []struct {
		name   string
		mutate func(*IntentArtifact)
		want   string
	}{
		{name: "duplicate constraint", mutate: func(intent *IntentArtifact) {
			intent.Constraints = []string{"preserve behavior", "preserve behavior"}
		}, want: "constraints must contain exact unique values"},
		{name: "blank ambiguity", mutate: func(intent *IntentArtifact) {
			intent.Ambiguities = []string{"  "}
		}, want: "ambiguities must contain exact unique values"},
		{name: "too many constraints", mutate: func(intent *IntentArtifact) {
			intent.Constraints = numberedIntentValues(maxIntentConstraints + 1)
		}, want: "constraints must contain at most"},
		{name: "too many ambiguities", mutate: func(intent *IntentArtifact) {
			intent.Ambiguities = numberedIntentValues(maxIntentAmbiguities + 1)
		}, want: "ambiguities must contain at most"},
		{name: "oversized constraint", mutate: func(intent *IntentArtifact) {
			intent.Constraints = []string{strings.Repeat("x", maxIntentProjectedTextBytes+1)}
		}, want: "constraints must contain exact unique values"},
		{name: "duplicate completion criterion", mutate: func(intent *IntentArtifact) {
			intent.CompletionCriteria = []string{"tests pass", "tests pass"}
		}, want: "completion_criteria must contain exact unique values"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			intent := base
			intent.Objectives = append([]Objective(nil), base.Objectives...)
			test.mutate(&intent)
			err := intent.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() err=%v, want %q", err, test.want)
			}
		})
	}
}

func numberedIntentValues(count int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = fmt.Sprintf("value %d", index+1)
	}
	return values
}
