package worker

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDesiredStateProductProviderSelectsByDeclarationNotCandidateOrder(t *testing.T) {
	t.Parallel()
	candidates := []assemblyline.ArtifactCandidateEvidence{
		{CandidateID: "ARTIFACT_CANDIDATE_1", Declarations: []string{"function Alpha: func Alpha() int"}},
		{CandidateID: "ARTIFACT_CANDIDATE_2", Declarations: []string{"function Obsolete: func Obsolete() int"}},
	}
	raw, err := json.Marshal(candidates)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := desiredStateProductSelectDeclarationCandidate(
		"BOUNDED_CANDIDATES:\n"+string(raw), "Obsolete",
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected != "ARTIFACT_CANDIDATE_2" {
		t.Fatalf("semantic candidate=%q want second opaque candidate", selected)
	}
}
