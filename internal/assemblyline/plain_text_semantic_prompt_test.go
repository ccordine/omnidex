package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestApplicationProductContextPromptExposesSemanticContentOnly(t *testing.T) {
	t.Parallel()
	const request = "Build a maintenance tracker for a repair shop."
	const fact = "The existing service stores work orders in PostgreSQL."
	context := ApplicationContext{
		Schema:        ApplicationContextSchemaV1,
		RequestSHA256: ExactObjectiveContextSHA(request),
		Facts: []ApplicationContextFact{{
			ID: "fact_001", Kind: ApplicationContextRepositoryFact,
			Authority: ApplicationContextEvidenceAuthority, NeedID: "need_internal_1",
			Value: fact, SourceID: "source_internal_1", SourceSHA256: ExactObjectiveContextSHA(fact),
		}},
	}

	product, err := BuildApplicationProductContextPrompt(
		ApplicationProductContextInput{UserRequest: request, Context: context},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, product, request, fact)
	assertPromptOmitsPacketState(t, product,
		ApplicationContextSchemaV1,
		context.RequestSHA256,
		"fact_001",
		"need_internal_1",
		"source_internal_1",
	)
}

func TestGroundedAnswerPromptsUsePlainTextAndOpaqueChoices(t *testing.T) {
	t.Parallel()
	context := plainTextPromptObjectiveContext("Repairs are scheduled by service date.")
	evidence := GroundedEvidenceCapsule{
		ID:   "evidence_internal_1",
		Text: "The inspection report records a two-day repair estimate.",
	}
	const requirement = "How long is the repair expected to take?"
	const paragraph = "The repair is expected to take two days."

	inventoryInput := GroundedAnswerParagraphInventoryInput{
		ExactRequirement: requirement,
		Context:          context,
		Evidence:         []GroundedEvidenceCapsule{evidence},
	}
	inventory, err := BuildGroundedAnswerParagraphInventoryPrompt(inventoryInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, inventory, requirement, context.Capsules[0].Content, evidence.Text, "List between 1 and 4 paragraphs")
	assertPromptOmitsPacketState(t, inventory,
		evidence.ID,
		context.Capsules[0].ContentSHA256,
		context.Capsules[0].Sources[0].CandidateID,
		GroundedAnswerParagraphInventorySchemaV1,
	)

	authorizationInput := GroundedAnswerParagraphAuthorizationInput{
		ExactRequirement: requirement,
		Context:          context,
		ParagraphText:    paragraph,
		Evidence:         []GroundedEvidenceCapsule{evidence},
	}
	authorization, err := BuildGroundedAnswerParagraphAuthorizationPrompt(authorizationInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, authorization, requirement, paragraph, evidence.Text, "A.", "B.", "Answer with A or B.")
	assertPromptOmitsPacketState(t, authorization,
		evidence.ID,
		string(GroundedParagraphResponsiveAndFullySupported),
		string(GroundedParagraphNotResponsiveOrUnsupported),
		context.Capsules[0].ContentSHA256,
	)

	relationInput := GroundedAnswerParagraphEvidenceRelationInput{
		ParagraphText: paragraph,
		Evidence:      evidence,
	}
	relation, err := BuildGroundedAnswerParagraphEvidenceRelationPrompt(relationInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, relation, paragraph, evidence.Text, "Answer with A or B.")
	assertPromptOmitsPacketState(t, relation,
		evidence.ID,
		string(GroundedEvidenceSupportsParagraph),
		string(GroundedEvidenceDoesNotSupport),
	)
}

func TestRoleplayAndWebPromptsOmitCodeOwnedPackets(t *testing.T) {
	t.Parallel()
	context := plainTextPromptObjectiveContext("Mara previously repaired the observatory clock.")
	evidence := GroundedEvidenceCapsule{
		ID:   "roleplay_evidence_internal_1",
		Text: "The almanac says the eclipse begins at midnight.",
	}
	identity := RoleplayResponseIdentity{
		CharacterName: "Mara",
		Summary:       "A careful astronomer who speaks plainly.",
		Voice:         "Measured and concise.",
	}
	const question = "When does the eclipse begin?"
	const paragraph = "The eclipse begins at midnight."
	roleplayInput := RoleplayGroundedResponseInput{
		ExactQuestion:    question,
		RoleplayIdentity: identity,
		RoleplayUserTurn: RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Ilan",
			ContributionKind: roleplay.UserContributionDialogue,
		},
		Context:           context,
		RealWorldEvidence: []GroundedEvidenceCapsule{evidence},
	}
	inventory, err := BuildRoleplayGroundedParagraphInventoryPrompt(roleplayInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, inventory, question, identity.CharacterName, identity.Summary, identity.Voice, context.Capsules[0].Content, evidence.Text, "List between 1 and 4 paragraphs")
	assertPromptOmitsPacketState(t, inventory,
		evidence.ID,
		context.Capsules[0].ContentSHA256,
		context.Capsules[0].Sources[0].CandidateID,
		RoleplayGroundedParagraphInventorySchemaV1,
	)

	authorizationInput := RoleplayGroundedParagraphAuthorizationInput{
		ExactQuestion: question, RoleplayIdentity: identity, Context: context,
		ParagraphText: paragraph, Evidence: []GroundedEvidenceCapsule{evidence},
	}
	authorization, err := BuildRoleplayGroundedParagraphAuthorizationPrompt(authorizationInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, authorization, question, paragraph, evidence.Text, "Answer with A or B.")
	assertPromptOmitsPacketState(t, authorization,
		evidence.ID,
		string(RoleplayGroundedParagraphResponsiveAndSupported),
		string(RoleplayGroundedParagraphNotAuthorized),
	)

	evidenceRelationInput := RoleplayGroundedEvidenceRelationInput{
		ExactQuestion: question, ParagraphText: paragraph, Evidence: evidence,
	}
	evidenceRelation, err := BuildRoleplayGroundedResponseEvidenceRelationPrompt(evidenceRelationInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, evidenceRelation, paragraph, evidence.Text, "Answer with A or B.")
	assertPromptOmitsPacketState(t, evidenceRelation,
		question,
		evidence.ID,
		string(RoleplayGroundedEvidenceSupportsParagraph),
		string(RoleplayGroundedEvidenceDoesNotSupport),
	)

	canonInput := RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind: RoleplayCanonSourceUserContribution, AttributedPersonaName: "Mara",
			ExactContribution: "I lock the observatory door.",
			PersonaKind:       roleplay.UserPersonaCharacter,
			ContributionKind:  roleplay.UserContributionDialogue,
		},
		Context: context,
	}
	presence, err := BuildRoleplayCanonFactPresencePrompt(canonInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, presence, "Mara", canonInput.Source.ExactContribution, context.Capsules[0].Content, "Answer with A or B.")
	assertPromptOmitsPacketState(t, presence,
		string(RoleplayCanonSourceUserContribution),
		RoleplayCanonContributionEstablishesFact,
		RoleplayCanonContributionEstablishesNoFact,
		context.Capsules[0].ContentSHA256,
		context.Capsules[0].Sources[0].CandidateID,
		RoleplayCanonFactPresenceSchemaV1,
	)

	canon, err := BuildRoleplayCanonFactInventoryPrompt(canonInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, canon, "Mara", canonInput.Source.ExactContribution, context.Capsules[0].Content, "List between 1 and")
	assertPromptOmitsPacketState(t, canon,
		string(RoleplayCanonSourceUserContribution),
		context.Capsules[0].ContentSHA256,
		context.Capsules[0].Sources[0].CandidateID,
		RoleplayCanonFactInventorySchemaV1,
	)

	webInput := WebRelevanceRelationInput{
		ExactQuestion: question,
		Context:       context,
		Candidate: WebRelevanceCandidate{
			CandidateID: "web_candidate_internal_1",
			Title:       "Eclipse timing",
			Snippet:     "A concise summary of the eclipse schedule.",
			Excerpt:     "The eclipse begins at midnight.",
		},
	}
	webPrompt, err := BuildWebRelevanceRelationPrompt(webInput)
	if err != nil {
		t.Fatal(err)
	}
	assertPromptContains(t, webPrompt, question, webInput.Candidate.Title, webInput.Candidate.Snippet, webInput.Candidate.Excerpt, "Answer with A or B.")
	assertPromptOmitsPacketState(t, webPrompt,
		webInput.Candidate.CandidateID,
		string(WebCandidateNotRelevant),
		context.Capsules[0].ContentSHA256,
	)
}

func plainTextPromptObjectiveContext(content string) ObjectiveContext {
	return ObjectiveContext{Capsules: []ObjectiveContextCapsule{{
		Sources: []ObjectiveContextSource{{
			Namespace:     "test.context",
			CandidateID:   "CTX_1",
			ContentSHA256: ExactObjectiveContextSHA(content),
		}},
		Content:       content,
		ContentSHA256: ExactObjectiveContextSHA(content),
	}}}
}

func assertPromptContains(t *testing.T, prompt string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(prompt, value) {
			t.Fatalf("prompt omitted semantic content %q:\n%s", value, prompt)
		}
	}
}

func assertPromptOmitsPacketState(t *testing.T, prompt string, values ...string) {
	t.Helper()
	for _, value := range append(values,
		"return only",
		"return exactly",
		"no json",
		"surrounding envelope",
		`{"`,
	) {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(value)) {
			t.Fatalf("prompt exposed packet or code-owned value %q:\n%s", value, prompt)
		}
	}
}
