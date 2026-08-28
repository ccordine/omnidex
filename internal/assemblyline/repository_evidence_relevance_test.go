package assemblyline

import (
	"fmt"
	"strings"
	"testing"
)

func TestRepositoryEvidenceRelevanceUsesOneRawOpaqueIDLeaf(t *testing.T) {
	t.Parallel()
	base := repositoryEvidenceRelevanceFixture()
	input := RepositoryEvidenceRelevanceLeafInput{
		ExactRequirement:    base.ExactRequirement,
		Candidates:          append([]RepositoryEvidenceCandidate(nil), base.Candidates...),
		SelectedEvidenceIDs: []string{},
		MaxSelections:       base.MaxSelections,
	}
	job, err := NewRepositoryEvidenceRelevanceLeafJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRepositoryEvidenceRelevanceLeaf {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, base.ExactRequirement) ||
		!strings.Contains(prompt, base.Candidates[0].Text) ||
		!strings.Contains(prompt, RepositoryEvidenceNoRelevantCandidate) {
		t.Fatalf("prompt lost bounded relevance authority: %q", prompt)
	}
	for _, forbidden := range []string{"evidence_ids array", `"schema"`, "outcome"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("raw relevance prompt exposes aggregate field %q: %s", forbidden, prompt)
		}
	}
	if got, err := DecodeRepositoryEvidenceRelevanceLeaf(input, "R02"); err != nil || got != "R02" {
		t.Fatalf("selected=%q error=%v", got, err)
	}
	if got, err := DecodeRepositoryEvidenceRelevanceLeaf(
		input, RepositoryEvidenceNoRelevantCandidate,
	); err != nil || got != RepositoryEvidenceNoRelevantCandidate {
		t.Fatalf("none=%q error=%v", got, err)
	}
}

func TestRepositoryEvidenceRelevanceLeafRejectsWrappersAndRetainedIDs(t *testing.T) {
	t.Parallel()
	base := repositoryEvidenceRelevanceFixture()
	base.MaxSelections = 2
	input := RepositoryEvidenceRelevanceLeafInput{
		ExactRequirement:    base.ExactRequirement,
		Candidates:          append([]RepositoryEvidenceCandidate(nil), base.Candidates...),
		SelectedEvidenceIDs: []string{"R01"},
		MaxSelections:       base.MaxSelections,
	}
	prompt, err := BuildRepositoryEvidenceRelevanceLeafPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, base.Candidates[0].Text) || !strings.Contains(prompt, base.Candidates[1].Text) {
		t.Fatalf("prompt did not project only remaining candidates: %s", prompt)
	}
	for _, raw := range []string{
		"R01", "R99", `{"evidence_id":"R02"}`, `"R02"`, " R02 ",
	} {
		if _, err := DecodeRepositoryEvidenceRelevanceLeaf(input, raw); err == nil {
			t.Fatalf("invalid raw relevance leaf accepted: %q", raw)
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
