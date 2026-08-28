package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayGroundedResponseUsesRawNarrativeAndPairwiseEvidenceLeaves(t *testing.T) {
	t.Parallel()
	input := roleplayGroundedFixture()
	job, err := NewRoleplayGroundedResponseTextJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRoleplayGroundedResponseText {
		t.Fatalf("kind=%q", job.Kind)
	}
	if !strings.Contains(prompt, "Ada") || !strings.Contains(prompt, "orbital period") ||
		!strings.Contains(prompt, input.RealWorldEvidence[0].Text) {
		t.Fatalf("grounded roleplay text authority was not projected: %s", prompt)
	}
	for _, forbidden := range []string{
		input.RealWorldEvidence[0].ID, `"paragraphs"`, `"evidence_ids"`, `"schema"`,
		`"roleplay_user_turn"`, "/research", "external_command", "web_research",
		"call a tool", "choose a tool", `"contribution_kind"`,
		"fictional_narrative_state", "unrelated crown archive", "meters", "inventory",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("model-visible text prompt exposes %q: %s", forbidden, prompt)
		}
	}

	text := "In this observatory, I'd call it about 365.25 days.\n\nThat is one trip around the Sun."
	decoded, err := DecodeRoleplayGroundedResponseTextLeaf(input, text)
	if err != nil || decoded != text {
		t.Fatalf("decoded=%q error=%v", decoded, err)
	}
	paragraphs, err := SplitRoleplayGroundedResponseParagraphs(decoded)
	if err != nil || len(paragraphs) != 2 {
		t.Fatalf("paragraphs=%#v error=%v", paragraphs, err)
	}

	relationInput := RoleplayGroundedEvidenceRelationInput{
		ExactQuestion: input.ExactQuestion,
		ParagraphText: paragraphs[0],
		Evidence:      input.RealWorldEvidence[0],
	}
	relationJob, err := NewRoleplayGroundedResponseEvidenceRelationJob(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	relationPrompt, err := RenderPortableJob(relationJob)
	if err != nil {
		t.Fatal(err)
	}
	if relationJob.Kind != WorkRoleplayGroundedResponseEvidenceRelation ||
		!strings.Contains(relationPrompt, paragraphs[0]) ||
		!strings.Contains(relationPrompt, input.RealWorldEvidence[0].Text) ||
		strings.Contains(relationPrompt, input.RealWorldEvidence[0].ID) ||
		strings.Contains(relationPrompt, input.RoleplayIdentity.CharacterName) {
		t.Fatalf("pairwise relation prompt is not minimal: %s", relationPrompt)
	}
	relation, err := DecodeRoleplayGroundedResponseEvidenceRelationLeaf(
		relationInput, string(RoleplayGroundedEvidenceSupportsParagraph),
	)
	if err != nil || relation != RoleplayGroundedEvidenceSupportsParagraph {
		t.Fatalf("relation=%q error=%v", relation, err)
	}
}

func TestRoleplayGroundedResponseRawLeavesRejectWrappersAndInvalidParagraphs(t *testing.T) {
	t.Parallel()
	input := roleplayGroundedFixture()
	for _, raw := range []string{
		`{"paragraphs":[{"text":"A year."}]}`,
		"A cited answer [1].",
		"A paragraph.\n\n\nAnother paragraph.",
		" A padded answer. ",
	} {
		if _, err := DecodeRoleplayGroundedResponseTextLeaf(input, raw); err == nil {
			t.Fatalf("invalid roleplay text leaf accepted: %q", raw)
		}
	}
	relationInput := RoleplayGroundedEvidenceRelationInput{
		ExactQuestion: input.ExactQuestion,
		ParagraphText: "Earth takes about 365.25 days to orbit the Sun.",
		Evidence:      input.RealWorldEvidence[0],
	}
	for _, raw := range []string{
		`{"relation":"SUPPORTS_PARAGRAPH"}`,
		"SUPPORTS_PARAGRAPH\nBecause it says so.",
		"SUPPORTED",
	} {
		if _, err := DecodeRoleplayGroundedResponseEvidenceRelationLeaf(
			relationInput, raw,
		); err == nil {
			t.Fatalf("invalid relation leaf accepted: %q", raw)
		}
	}
}

func TestRoleplayGroundedResponseAssemblyRetainsOnlyCodeBoundEvidenceIDs(t *testing.T) {
	t.Parallel()
	input := roleplayGroundedFixture()
	decision, err := AssembleRoleplayGroundedResponseDecision(
		input,
		[]RoleplayGroundedParagraph{{
			Text:        "Earth's orbit takes approximately 365.25 days.",
			EvidenceIDs: []string{input.RealWorldEvidence[0].ID},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Schema != RoleplayGroundedResponseSchemaV1 ||
		len(decision.Paragraphs) != 1 ||
		decision.Paragraphs[0].EvidenceIDs[0] != input.RealWorldEvidence[0].ID {
		t.Fatalf("decision=%#v", decision)
	}
	if _, err := AssembleRoleplayGroundedResponseDecision(
		input,
		[]RoleplayGroundedParagraph{{Text: "Unsupported answer.", EvidenceIDs: []string{"missing"}}},
	); err == nil {
		t.Fatal("unavailable model-authored evidence ID was assembled")
	}
}

func roleplayGroundedFixture() RoleplayGroundedResponseInput {
	contextText := "Ada is answering from the observatory."
	contextSource := "The current scene is the observatory."
	return RoleplayGroundedResponseInput{
		ExactQuestion: "What is Earth's orbital period?",
		RoleplayIdentity: RoleplayResponseIdentity{
			CharacterName: "Ada", Summary: "A careful astronomer.", Voice: "Measured",
		},
		RoleplayUserTurn: RoleplayUserTurnProjection{
			PersonaKind:      roleplay.UserPersonaNarrator,
			PersonaName:      roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionCommand,
		},
		Context: ObjectiveContext{Capsules: []ObjectiveContextCapsule{{
			Sources: []ObjectiveContextSource{{
				Namespace: "roleplay_scene", CandidateID: "CTX_1",
				ContentSHA256: ExactObjectiveContextSHA(contextSource),
			}},
			Content: contextText, ContentSHA256: ExactObjectiveContextSHA(contextText),
		}}},
		RealWorldEvidence: []GroundedEvidenceCapsule{{
			ID: "doc-1", Text: "Earth's orbital period is approximately 365.25 days.",
		}},
	}
}
