package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRepositoryGroundedReviewUsesRawDetailThenRawKind(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	detailJob, err := NewRepositoryGroundedIssueDetailJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(detailJob)
	if err != nil {
		t.Fatal(err)
	}
	if detailJob.Kind != WorkRepositoryGroundedIssueDetail {
		t.Fatalf("detail kind=%q", detailJob.Kind)
	}
	if !strings.Contains(prompt, input.AnswerText) ||
		!strings.Contains(prompt, input.Evidence[0].Text) ||
		!strings.Contains(prompt, RepositoryGroundedNoIssue) {
		t.Fatalf("detail prompt lost exact comparison authority: %s", prompt)
	}
	for _, forbidden := range []string{input.RequirementID, input.EvidenceIDs[0], `"outcome"`, `"issue_kind"`} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("detail prompt exposed code-owned field %q: %s", forbidden, prompt)
		}
	}
	detail, err := DecodeRepositoryGroundedIssueDetailLeaf(
		input, "The ownership claim is absent from the cited evidence.",
	)
	if err != nil {
		t.Fatal(err)
	}
	kindInput := RepositoryGroundedIssueKindLeafInput{Review: input, Detail: detail}
	kindJob, err := NewRepositoryGroundedIssueKindJob(kindInput)
	if err != nil {
		t.Fatal(err)
	}
	kindPrompt, err := RenderPortableJob(kindJob)
	if err != nil {
		t.Fatal(err)
	}
	if kindJob.Kind != WorkRepositoryGroundedIssueKind ||
		!strings.Contains(kindPrompt, detail) {
		t.Fatalf("kind=%q prompt=%s", kindJob.Kind, kindPrompt)
	}
	kind, err := DecodeRepositoryGroundedIssueKindLeaf(
		kindInput, string(RepositoryGroundedUnsupportedClaim),
	)
	if err != nil || kind != RepositoryGroundedUnsupportedClaim {
		t.Fatalf("kind=%q error=%v", kind, err)
	}
	decision, err := AssembleRepositoryGroundedReviewDecision(input, detail, kind)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Outcome != RepositoryGroundedReviewIssue ||
		decision.Detail != detail || decision.IssueKind != kind {
		t.Fatalf("assembled review=%#v", decision)
	}
}

func TestRepositoryGroundedNoIssueIsDecodedAndAssembledByCode(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	detail, err := DecodeRepositoryGroundedIssueDetailLeaf(
		input, RepositoryGroundedNoIssue,
	)
	if err != nil || detail != "" {
		t.Fatalf("detail=%q error=%v", detail, err)
	}
	decision, err := AssembleRepositoryGroundedReviewDecision(input, detail, "")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Schema != RepositoryGroundedReviewSchemaV1 ||
		decision.Outcome != RepositoryGroundedReviewNone ||
		decision.IssueKind != "" || decision.Detail != "" {
		t.Fatalf("assembled no-issue review=%#v", decision)
	}
}

func TestRepositoryGroundedReviewRawLeavesRejectAggregateAndAmbiguousValues(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	for _, raw := range []string{
		`{"outcome":"none"}`,
		"line one\nline two",
		" detail ",
	} {
		if _, err := DecodeRepositoryGroundedIssueDetailLeaf(input, raw); err == nil {
			t.Fatalf("invalid detail leaf accepted: %q", raw)
		}
	}
	kindInput := RepositoryGroundedIssueKindLeafInput{
		Review: input,
		Detail: "The ownership claim is unsupported.",
	}
	for _, raw := range []string{
		RepositoryGroundedNoIssue,
		`{"issue_kind":"unsupported_claim"}`,
		"issue",
		"unsupported_claim\nexplanation",
	} {
		if _, err := DecodeRepositoryGroundedIssueKindLeaf(kindInput, raw); err == nil {
			t.Fatalf("invalid issue-kind leaf accepted: %q", raw)
		}
	}
	if _, err := AssembleRepositoryGroundedReviewDecision(
		input, "The ownership claim is unsupported.", "",
	); err == nil {
		t.Fatal("detail without a registered kind was assembled")
	}
}

func TestRepositoryGroundedReviewRequiresExactlyCitedEvidence(t *testing.T) {
	t.Parallel()
	input := repositoryGroundedReviewFixture()
	input.Evidence = append(input.Evidence, GroundedEvidenceCapsule{
		ID: "R02", Text: "uncited evidence",
	})
	if err := input.Validate(); err == nil {
		t.Fatal("uncited evidence was exposed to grounded review")
	}
	input = repositoryGroundedReviewFixture()
	input.EvidenceIDs = []string{"R02"}
	if err := input.Validate(); err == nil {
		t.Fatal("unprojected cited evidence was accepted")
	}
	input = repositoryGroundedReviewFixture()
	input.EvidenceIDs = append(input.EvidenceIDs, "R02")
	input.Evidence = append(input.Evidence, GroundedEvidenceCapsule{
		ID: "R02", Text: input.Evidence[0].Text,
	})
	if err := input.Validate(); err == nil {
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

func repositoryGroundedReviewFixture() RepositoryGroundedReviewInput {
	return RepositoryGroundedReviewInput{
		RequirementID:    "requirement-17",
		ExactRequirement: "Which component owns dispatch?",
		Context:          minifiedObjectiveContext("The earlier result discussed dispatch ownership."),
		AnswerText:       "ScheduleDispatch owns dispatch.",
		EvidenceIDs:      []string{"R01"},
		Evidence: []GroundedEvidenceCapsule{{
			ID: "R01", Text: "func ScheduleDispatch() starts dispatch.",
		}},
	}
}
