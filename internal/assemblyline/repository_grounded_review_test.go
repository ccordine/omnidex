package assemblyline

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryGroundedReviewReturnsOnlyNoneOrOneIssue(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	job, err := NewRepositoryGroundedReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRepositoryGroundedReview {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.AnswerText) || !strings.Contains(prompt, input.Evidence[0].Text) {
		t.Fatalf("review prompt lost exact claim or cited evidence: %q", prompt)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("schema is not closed: %#v", schema)
	}
	assertExactJSONFields(t, reflect.TypeOf(input), []string{
		"requirement_id", "exact_requirement", "objective_context",
		"answer_text", "evidence_ids", "evidence",
	})
	none := RepositoryGroundedReviewDecision{
		Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewNone,
	}
	if err := none.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	issue := RepositoryGroundedReviewDecision{
		Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewIssue,
		IssueKind: RepositoryGroundedUnsupportedClaim, Detail: "The ownership claim is absent from R01.",
	}
	if err := issue.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	assertExactJSONFields(t, reflect.TypeOf(RepositoryGroundedReviewDecision{}), []string{"schema", "outcome", "issue_kind", "detail"})
}

func TestRepositoryGroundedReviewRequiresExactlyTheCitedEvidence(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	input.Evidence = append(input.Evidence, GroundedEvidenceCapsule{ID: "R02", Text: "uncited evidence"})
	if _, err := NewRepositoryGroundedReviewJob(input); err == nil {
		t.Fatal("uncited evidence was exposed to independent review")
	}
	input = repositoryGroundedReviewFixture()
	input.EvidenceIDs = []string{"R02"}
	if _, err := NewRepositoryGroundedReviewJob(input); err == nil {
		t.Fatal("unprojected cited evidence was accepted")
	}
	input = repositoryGroundedReviewFixture()
	input.EvidenceIDs = append(input.EvidenceIDs, "R02")
	input.Evidence = append(input.Evidence, GroundedEvidenceCapsule{ID: "R02", Text: input.Evidence[0].Text})
	if _, err := NewRepositoryGroundedReviewJob(input); err == nil {
		t.Fatal("duplicate evidence text was accepted under another ID")
	}
}

func TestRepositoryGroundedReviewRejectsRetiredAdvisoryProjection(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(repositoryGroundedReviewFixture())
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw[:len(raw)-1], []byte(`,"advisory_capsules":[]}`)...)
	var decoded RepositoryGroundedReviewInput
	if err := decodePortablePayload(raw, &decoded); err == nil ||
		!strings.Contains(err.Error(), "advisory_capsules") {
		t.Fatalf("retired advisory projection error=%v", err)
	}
}

func TestRepositoryGroundedReviewRejectsAmbiguousIssueState(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	tests := map[string]RepositoryGroundedReviewDecision{
		"none with kind": {Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewNone, IssueKind: RepositoryGroundedUnsupportedClaim},
		"none detail":    {Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewNone, Detail: "wrong"},
		"issue no kind":  {Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewIssue, Detail: "wrong"},
		"issue no detail": {Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewIssue,
			IssueKind: RepositoryGroundedUnsupportedClaim},
		"unknown issue": {Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewIssue,
			IssueKind: "invented", Detail: "wrong"},
		"multiline": {Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewIssue,
			IssueKind: RepositoryGroundedUnsupportedClaim, Detail: "line one\nline two"},
	}
	for name, decision := range tests {
		decision := decision
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := decision.ValidateFor(input); err == nil {
				t.Fatalf("accepted %#v", decision)
			}
		})
	}
}

func TestRepositoryGroundedReviewDecodeRejectsExtraOrDuplicateState(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	valid := fmt.Sprintf(
		`{"schema":%q,"outcome":"none","issue_kind":"","detail":""}`,
		RepositoryGroundedReviewSchemaV1,
	)
	if _, err := DecodeRepositoryGroundedReviewDecision(input, valid); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		strings.TrimSuffix(valid, "}") + `,"extra":true}`,
		fmt.Sprintf(`{"schema":%q,"schema":%q,"outcome":"none","issue_kind":"","detail":""}`,
			RepositoryGroundedReviewSchemaV1, RepositoryGroundedReviewSchemaV1),
	} {
		if _, err := DecodeRepositoryGroundedReviewDecision(input, raw); err == nil {
			t.Fatalf("malformed review accepted: %s", raw)
		}
	}
}

func repositoryGroundedReviewFixture() RepositoryGroundedReviewInput {
	return RepositoryGroundedReviewInput{
		RequirementID: "requirement-17", ExactRequirement: "Which component owns dispatch?",
		Context:    minifiedObjectiveContext("The earlier result discussed dispatch ownership."),
		AnswerText: "ScheduleDispatch owns dispatch.", EvidenceIDs: []string{"R01"},
		Evidence: []GroundedEvidenceCapsule{{ID: "R01", Text: "func ScheduleDispatch() starts dispatch."}},
	}
}

func TestRepositoryGroundedReviewRejectsOversizedCandidate(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	input.AnswerText = strings.Repeat("a", maxGroundedAnswerTextBytes+1)
	if _, err := NewRepositoryGroundedReviewJob(input); err == nil {
		t.Fatal("oversized answer was copied into review")
	}
	issue := RepositoryGroundedReviewDecision{
		Schema: RepositoryGroundedReviewSchemaV1, Outcome: RepositoryGroundedReviewIssue,
		IssueKind: RepositoryGroundedUnsupportedClaim, Detail: strings.Repeat("x", maxRepositoryGroundedReviewDetailBytes+1),
	}
	if err := issue.ValidateFor(repositoryGroundedReviewFixture()); err == nil {
		t.Fatal(fmt.Sprintf("oversized issue accepted: %#v", issue))
	}
}
