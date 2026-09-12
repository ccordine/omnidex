package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestObjectiveIdentityBelongsToJobGeneration(t *testing.T) {
	authority := turnAuthority{JobID: 17, Generation: 2, Instruction: "Answer the current question."}
	want := "objective-17-2"
	if got := objectiveTurnID(authority); got != want {
		t.Fatalf("objective identity = %q, want %q", got, want)
	}
	authority.Context = assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{{
		Content: "One newly acquired fact.",
		Sources: []assemblyline.ObjectiveContextSource{{Namespace: "repository", CandidateID: "CTX_1"}},
	}}}
	if got := objectiveTurnID(authority); got != want {
		t.Fatalf("context acquisition changed objective identity to %q", got)
	}
	if got := objectiveRequirementID(want); got != "objective-17-2-requirement" {
		t.Fatalf("requirement identity = %q", got)
	}
	authority.Generation++
	if objectiveTurnID(authority) == want {
		t.Fatal("another generation reused the old objective identity")
	}
	authority.Generation--
	authority.JobID++
	if objectiveTurnID(authority) == want {
		t.Fatal("another job reused the old objective identity")
	}
}

func TestTurnAuthorityRequiresPersistedGeneration(t *testing.T) {
	if _, err := newTurnAuthority(model.Job{ID: 17}); err == nil {
		t.Fatal("turn authority accepted an absent generation")
	}
}
