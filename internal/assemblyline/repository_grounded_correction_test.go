package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryGroundedCorrectionReturnsOneChangedTextLeaf(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedCorrectionFixture()
	job, err := NewRepositoryGroundedCorrectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRepositoryGroundedCorrection {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.CurrentText) || !strings.Contains(prompt, input.Evidence[0].Text) {
		t.Fatalf("correction prompt lost retained candidate or evidence: %q", prompt)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("schema is not closed: %#v", schema)
	}
	properties := schema["properties"].(map[string]any)
	textSchema := properties["text"].(map[string]any)
	if _, finiteGrammarBound := textSchema["maxLength"]; finiteGrammarBound {
		t.Fatalf("repository correction schema encodes the code-owned byte ceiling: %#v", textSchema)
	}
	assertExactJSONFields(t, reflect.TypeOf(input), []string{
		"requirement_id", "exact_requirement", "objective_context",
		"current_text", "evidence_ids", "evidence", "issue",
	})
	decision := RepositoryGroundedCorrectionDecision{Text: "ScheduleDispatch starts the dispatch timer."}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	assertExactJSONFields(t, reflect.TypeOf(RepositoryGroundedCorrectionDecision{}), []string{"text"})
}

func TestRepositoryGroundedCorrectionRetainsEvidenceIDsAndRejectsNoop(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedCorrectionFixture()
	decision := RepositoryGroundedCorrectionDecision{Text: input.CurrentText}
	if err := decision.ValidateFor(input); err == nil {
		t.Fatal("no-op correction was accepted")
	}
	input.Issue.Outcome = RepositoryGroundedReviewNone
	input.Issue.IssueKind = ""
	input.Issue.Detail = ""
	if _, err := NewRepositoryGroundedCorrectionJob(input); err == nil {
		t.Fatal("correction without an exact review issue was accepted")
	}
	input = repositoryGroundedCorrectionFixture()
	input.EvidenceIDs = []string{"R02"}
	if _, err := NewRepositoryGroundedCorrectionJob(input); err == nil {
		t.Fatal("correction accepted evidence IDs not retained in its exact evidence")
	}
}

func TestRepositoryGroundedCorrectionRejectsExtraOrOversizedLeaf(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedCorrectionFixture()
	raw := `{"text":"changed","evidence_ids":["R01"]}`
	if _, err := DecodeRepositoryGroundedCorrectionDecision(input, raw); err == nil {
		t.Fatal("correction changed more than the text leaf")
	}
	if _, err := DecodeRepositoryGroundedCorrectionDecision(input, `{"text":"changed","text":"changed again"}`); err == nil {
		t.Fatal("correction repeated the only mutable leaf")
	}
	if _, err := DecodeRepositoryGroundedCorrectionDecision(input, `{"text":"`+strings.Repeat("x", maxGroundedAnswerTextBytes+1)+`"}`); err == nil {
		t.Fatal("oversized correction leaf accepted")
	}
}

func repositoryGroundedCorrectionFixture() RepositoryGroundedCorrectionInput {
	return RepositoryGroundedCorrectionInput{
		RequirementID: "requirement-17", ExactRequirement: "Which component owns dispatch?",
		Context:     minifiedObjectiveContext("The earlier result discussed dispatch ownership."),
		CurrentText: "Scheduler owns dispatch.", EvidenceIDs: []string{"R01"},
		Evidence: []GroundedEvidenceCapsule{{ID: "R01", Text: "func ScheduleDispatch() starts dispatch."}},
		Issue: RepositoryGroundedReviewDecision{
			Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewIssue,
			IssueKind: RepositoryGroundedUnsupportedClaim, Detail: "The component name is unsupported.",
		},
	}
}
