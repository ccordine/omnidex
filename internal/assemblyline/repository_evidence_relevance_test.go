package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func TestRepositoryEvidenceRelevanceUsesOneCandidateBoundRelation(t *testing.T) {
	t.Parallel()
	base := repositoryEvidenceRelevanceFixture()
	input := RepositoryEvidenceRelevanceRelationInput{
		ExactRequirement: base.ExactRequirement,
		Candidate:        base.Candidates[1],
	}
	job, err := NewRepositoryEvidenceRelevanceRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRepositoryEvidenceRelevanceRelation {
		t.Fatalf("kind=%q", job.Kind)
	}
	if maximum, err := PortableResponseMaximumBytesForJob(job); err != nil ||
		maximum != len(RepositoryEvidenceNotDirectlyRelevant) {
		t.Fatalf("response maximum=%d error=%v", maximum, err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, base.ExactRequirement) ||
		!strings.Contains(prompt, base.Candidates[1].Text) ||
		strings.Contains(prompt, base.Candidates[0].Text) ||
		strings.Contains(prompt, base.Candidates[1].EvidenceID) {
		t.Fatalf("prompt lost bounded relevance authority: %q", prompt)
	}
	for _, forbidden := range []string{
		"evidence_ids array", `"schema"`, "most directly", "none of the remaining",
		"ranking", "selection", "completeness", "workflow", "continues",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("relation prompt exposes ranking/control authority %q: %s", forbidden, prompt)
		}
	}
	got, err := DecodeRepositoryEvidenceRelevanceRelationResult(
		input, RepositoryEvidenceDirectlyRelevant,
	)
	if err != nil || got.Relation != RepositoryEvidenceDirectlyRelevant {
		t.Fatalf("relation=%+v error=%v", got, err)
	}
	other := input
	other.Candidate = base.Candidates[0]
	if err := got.ValidateFor(other); err == nil {
		t.Fatal("candidate-bound relevance relation replayed against another candidate")
	}
}

func TestRepositoryEvidenceRelevanceRelationRejectsWrappersAndControlValues(t *testing.T) {
	t.Parallel()
	base := repositoryEvidenceRelevanceFixture()
	input := RepositoryEvidenceRelevanceRelationInput{
		ExactRequirement: base.ExactRequirement,
		Candidate:        base.Candidates[0],
	}
	for _, raw := range []string{
		"R01", "NO_RELEVANT_EVIDENCE", `{"relation":"DIRECTLY_RELEVANT"}`,
		`"DIRECTLY_RELEVANT"`, " DIRECTLY_RELEVANT ",
	} {
		if _, err := DecodeRepositoryEvidenceRelevanceRelationResult(input, raw); err == nil {
			t.Fatalf("invalid raw relevance relation accepted: %q", raw)
		}
	}
}

func TestRepositoryEvidenceRelevanceAssemblyIsCodeOwned(t *testing.T) {
	t.Parallel()
	input := repositoryEvidenceRelevanceFixture()
	selected, err := AssembleRepositoryEvidenceRelevanceDecision(input, []string{"R02"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Schema != RepositoryEvidenceRelevanceSchemaV1 ||
		len(selected.EvidenceIDs) != 1 || selected.EvidenceIDs[0] != "R02" {
		t.Fatalf("selected=%#v", selected)
	}
	none, err := AssembleRepositoryEvidenceRelevanceDecision(input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if none.EvidenceIDs == nil || len(none.EvidenceIDs) != 0 {
		t.Fatalf("empty code-owned selection=%#v", none)
	}
	if _, err := AssembleRepositoryEvidenceRelevanceDecision(input, []string{"R99"}); err == nil {
		t.Fatal("unprojected evidence ID was assembled")
	}
	input.MaxSelections = 2
	if _, err := AssembleRepositoryEvidenceRelevanceDecision(input, []string{"R02", "R01"}); err == nil {
		t.Fatal("reordered evidence IDs were assembled")
	}
}

func TestRepositoryEvidenceRelevanceRejectsUnboundedLeafAuthority(t *testing.T) {
	t.Parallel()
	base := repositoryEvidenceRelevanceFixture()
	base.ExactRequirement = strings.Repeat("q", maxGroundedRequirementBytes+1)
	if err := base.Validate(); err == nil {
		t.Fatal("oversized exact requirement accepted")
	}
	base = repositoryEvidenceRelevanceFixture()
	base.Candidates[0].Text = strings.Repeat("e", maxGroundedEvidenceTextBytes+1)
	if err := base.Validate(); err == nil {
		t.Fatal("oversized evidence candidate accepted")
	}
	base = repositoryEvidenceRelevanceFixture()
	for index := 0; index < maxRepositoryRelevanceCandidates; index++ {
		base.Candidates = append(base.Candidates, RepositoryEvidenceCandidate{
			EvidenceID: fmt.Sprintf("E%d", index),
			Text:       "bounded unique evidence " + fmt.Sprint(index),
		})
	}
	if err := base.Validate(); err == nil {
		t.Fatal("unbounded candidate count accepted")
	}
}

func repositoryEvidenceRelevanceFixture() RepositoryEvidenceRelevanceInput {
	return RepositoryEvidenceRelevanceInput{
		ExactRequirement: "Explain which component owns dispatch timing.",
		Candidates: []RepositoryEvidenceCandidate{
			{EvidenceID: "R01", Text: "func ScheduleDispatch() starts the delivery timer."},
			{EvidenceID: "R02", Text: "func RecordDelivery() stores the final delivery receipt."},
		},
		MaxSelections: 1,
	}
}
