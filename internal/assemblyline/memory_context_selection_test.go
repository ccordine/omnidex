package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestMemoryContextSelectionReturnsOnlyAvailableIDs(t *testing.T) {
	input := memoryContextSelectionFixture()
	job, err := NewMemoryContextSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkMemoryContextSelection {
		t.Fatalf("kind=%q", job.Kind)
	}
	decision, err := DecodeMemoryContextSelectionDecision(input,
		`{"schema":"omnidex.memory-context-selection.v1","referenced_memory_ids":[92]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.ReferencedMemoryIDs) != 1 || decision.ReferencedMemoryIDs[0] != 92 {
		t.Fatalf("decision=%#v", decision)
	}
	for _, raw := range []string{
		`{"schema":"omnidex.memory-context-selection.v1","referenced_memory_ids":[999]}`,
		`{"schema":"omnidex.memory-context-selection.v1","referenced_memory_ids":[91,91]}`,
		`{"schema":"omnidex.memory-context-selection.v1","referenced_memory_ids":[],"content":"invented"}`,
	} {
		if _, err := DecodeMemoryContextSelectionDecision(input, raw); err == nil {
			t.Fatalf("accepted invalid ID-only decision %s", raw)
		}
	}
}

func TestMemoryContextSelectionRejectsForgedOrUnboundedCandidates(t *testing.T) {
	input := memoryContextSelectionFixture()
	input.CandidateAuthorities[0].ContentSHA256 = strings.Repeat("0", 64)
	if _, err := NewMemoryContextSelectionJob(input); err == nil {
		t.Fatal("accepted forged candidate content hash")
	}
	input = memoryContextSelectionFixture()
	input.CandidateAuthorities = append(input.CandidateAuthorities,
		make([]MemoryContextCandidate, MaxMemoryContextCandidateAuthorities)...)
	if _, err := NewMemoryContextSelectionJob(input); err == nil {
		t.Fatal("accepted more than eight candidates")
	}
	input = memoryContextSelectionFixture()
	input.CandidateAuthorities[0].Content = strings.Repeat("x", MaxSelectedMemoryProjectionBytes+1)
	input.CandidateAuthorities[0].ContentSHA256 = ExactObjectiveContextSHA(input.CandidateAuthorities[0].Content)
	decision := MemoryContextSelectionDecision{
		Schema: MemoryContextSelectionSchemaV1, ReferencedMemoryIDs: []int64{91},
	}
	if err := decision.ValidateFor(input); err == nil {
		t.Fatal("accepted oversized selected projection")
	}
}

func TestObjectiveContextRejectsRewrittenMemoryAndCrossJobReplan(t *testing.T) {
	context := ObjectiveContext{
		MemoryAuthorities: []ObjectiveMemoryAuthority{{
			MemoryID: 91, Kind: model.MemoryKindReference, Content: "Exact remembered fact.",
			ContentSHA256: ExactObjectiveContextSHA("Exact remembered fact."),
		}},
		ReplanAuthority: &ObjectiveReplanAuthority{
			JobID: 7, Generation: 2, Feedback: "Keep the same instruction; fix the failing property.",
			FeedbackSHA256: ExactObjectiveContextSHA("Keep the same instruction; fix the failing property."),
		},
	}
	if err := context.Validate(); err != nil {
		t.Fatal(err)
	}
	context.MemoryAuthorities[0].Content = "rewritten"
	if err := context.Validate(); err == nil {
		t.Fatal("accepted rewritten memory capsule")
	}
	context = CloneObjectiveContext(ObjectiveContext{ReplanAuthority: &ObjectiveReplanAuthority{
		JobID: 0, Generation: 2, Feedback: "Exact feedback.",
		FeedbackSHA256: ExactObjectiveContextSHA("Exact feedback."),
	}})
	if err := context.Validate(); err == nil {
		t.Fatal("accepted replan feedback without same-job identity")
	}
}

func memoryContextSelectionFixture() MemoryContextSelectionInput {
	contentA := "Use the deployment constraint recorded earlier."
	contentB := "The database owner requires immutable evidence."
	return MemoryContextSelectionInput{
		ExactInstruction: "Apply the relevant constraint.",
		MaxSelectedBytes: MaxSelectedMemoryProjectionBytes,
		CandidateAuthorities: []MemoryContextCandidate{
			{MemoryID: 91, Kind: model.MemoryKindInstruction, Content: contentA, ContentSHA256: ExactObjectiveContextSHA(contentA)},
			{MemoryID: 92, Kind: model.MemoryKindReference, Content: contentB, ContentSHA256: ExactObjectiveContextSHA(contentB)},
		},
	}
}
