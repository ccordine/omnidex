package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestSemanticPlanningObjectCandidatePreservesOriginalContextCall(t *testing.T) {
	candidate, err := assemblyline.NewContextCandidateAuthority("session_turn", "CTX_1", "The current application bookmarks passages.")
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.ContextRelevanceRelationInput{ExactInstruction: "Preserve the passage bookmarks.", Candidate: candidate, KnownArtifactPaths: []string{}}
	job, err := assemblyline.NewContextRelevanceRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	var observations []model.PlanningActivity
	calls := 0
	runtime := typedWorkerRuntime{Context: t.Context(), ObservePlanning: func(activity model.PlanningActivity) { observations = append(observations, activity) }, Execute: func(actual assemblyline.PortableJob, name string) (assemblyline.PortableResult, error) {
		calls++
		if actual.Key() != job.Key() || name != "fixture-model" {
			t.Fatal("display changed context call")
		}
		return assemblyline.PortableResult{Candidate: "A"}, nil
	}}
	_, err = runObjectiveRawLeafWorkerCall(runtime, "fixture-model", string(job.Kind), job, func(raw string) (assemblyline.ContextRelevanceRelationResult, error) {
		return assemblyline.DecodeContextRelevanceRelationResult(input, raw)
	})
	if err != nil || calls != 1 || len(observations) != 2 {
		t.Fatalf("display interrupted valid context work: calls=%d observations=%v error=%v", calls, observations, err)
	}
	if observations[0].Subject != "session_turn/CTX_1" || observations[1].Result != observations[0].Options[0] {
		t.Fatalf("context source identity or actual decision lost: %+v", observations)
	}
}
