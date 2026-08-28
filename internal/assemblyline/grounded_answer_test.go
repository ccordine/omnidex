package assemblyline

import (
	"strings"
	"testing"
)

func TestGroundedAnswerLeavesSeparateTextFromEvidenceBinding(t *testing.T) {
	input := groundedAnswerFixture()
	textInput := GroundedAnswerTextInput{
		ExactRequirement: input.ExactRequirement,
		Context:          input.Context, Evidence: input.Evidence,
	}
	textJob, err := NewGroundedAnswerTextJob(textInput)
	if err != nil {
		t.Fatal(err)
	}
	if textJob.Kind != WorkGroundedAnswerText {
		t.Fatalf("kind=%q", textJob.Kind)
	}
	prompt, err := BuildGroundedAnswerTextPrompt(textInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.Evidence[0].Text) || strings.Contains(prompt, input.Evidence[0].ID) {
		t.Fatalf("text prompt lost evidence or exposed its code-owned ID: %q", prompt)
	}
	text, err := DecodeGroundedAnswerTextDecision(
		textInput, "The dispatch interval controls invitation timing.",
	)
	if err != nil {
		t.Fatal(err)
	}

	relationInput := GroundedAnswerEvidenceRelationInput{
		ExactRequirement: input.ExactRequirement,
		Context:          input.Context, AnswerText: text.Text, Evidence: input.Evidence[0],
	}
	relationJob, err := NewGroundedAnswerEvidenceRelationJob(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	if relationJob.Kind != WorkGroundedAnswerEvidenceRelation {
		t.Fatalf("kind=%q", relationJob.Kind)
	}
	relationPrompt, err := BuildGroundedAnswerEvidenceRelationPrompt(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(relationPrompt, relationInput.Evidence.ID) {
		t.Fatalf("pairwise relation prompt exposed code-owned evidence ID: %q", relationPrompt)
	}
	relation, err := DecodeGroundedAnswerEvidenceRelationDecision(
		relationInput, string(GroundedEvidenceSupportsAnswer),
	)
	if err != nil || relation.Relation != GroundedEvidenceSupportsAnswer {
		t.Fatalf("relation=%+v err=%v", relation, err)
	}
}

func TestGroundedAnswerLeafDecodersRejectStructuredOrCompositeResults(t *testing.T) {
	input := groundedAnswerFixture()
	textInput := GroundedAnswerTextInput{
		ExactRequirement: input.ExactRequirement,
		Context:          input.Context, Evidence: input.Evidence,
	}
	for _, raw := range []string{
		`{"text":"answer"}`,
		`["answer"]`,
		`"answer"`,
		"```\nanswer\n```",
	} {
		if _, err := DecodeGroundedAnswerTextDecision(textInput, raw); err == nil {
			t.Fatalf("structured text result accepted: %q", raw)
		}
	}
	relationInput := GroundedAnswerEvidenceRelationInput{
		ExactRequirement: input.ExactRequirement,
		Context:          input.Context, AnswerText: "Answer.", Evidence: input.Evidence[0],
	}
	for _, raw := range []string{"supports", "SUPPORTS_ANSWER\ncomment", `{"relation":"SUPPORTS_ANSWER"}`} {
		if _, err := DecodeGroundedAnswerEvidenceRelationDecision(relationInput, raw); err == nil {
			t.Fatalf("invalid relation accepted: %q", raw)
		}
	}
}

func TestGroundedAnswerDecisionIsAssembledByCode(t *testing.T) {
	input := groundedAnswerFixture()
	decision := GroundedAnswerDecision{
		Schema: GroundedAnswerSchemaV1, RequirementID: input.RequirementID,
		Text:        "The dispatch interval controls invitation timing.",
		EvidenceIDs: []string{"E17"},
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	decision.EvidenceIDs = []string{"E99"}
	if err := decision.ValidateFor(input); err == nil {
		t.Fatal("code-owned assembly accepted an unprojected evidence ID")
	}
}

func groundedAnswerFixture() GroundedAnswerInput {
	return GroundedAnswerInput{
		RequirementID:    "R17",
		ExactRequirement: "Explain which setting controls invitation timing.",
		Evidence: []GroundedEvidenceCapsule{
			{ID: "E17", Text: "ClientDeliveryConfig declares dispatch interval."},
			{ID: "E31", Text: "InvitationScheduler reads the dispatch interval."},
		},
	}
}
