package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayGroundedResponseReceivesOnlyIdentityMinifiedContextAndEvidence(t *testing.T) {
	t.Parallel()
	input := roleplayGroundedFixture()
	job, err := NewRoleplayGroundedResponseJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRoleplayGroundedResponse ||
		!strings.Contains(prompt, "Ada") || !strings.Contains(prompt, "orbital period") {
		t.Fatalf("grounded roleplay input was not projected: %s", prompt)
	}
	assertExactObjectSchemaFields(t, schema, []string{"schema", "paragraphs"})
	properties := schema["properties"].(map[string]any)
	paragraphs := properties["paragraphs"].(map[string]any)
	paragraph := paragraphs["items"].(map[string]any)
	paragraphProperties := paragraph["properties"].(map[string]any)
	textSchema := paragraphProperties["text"].(map[string]any)
	if _, providerHostileBound := textSchema["maxLength"]; providerHostileBound {
		t.Fatalf("roleplay response schema contains a provider-hostile grammar repetition: %#v", textSchema)
	}
	assertExactJSONFields(t, reflect.TypeOf(input), []string{
		"exact_question", "roleplay_identity", "roleplay_user_turn", "objective_context", "real_world_evidence",
	})
	lower := strings.ToLower(prompt)
	for _, forbidden := range []string{
		"/research", "external_command", "web_research", "resolver", "catalog", "tool schema",
		"call a tool", "choose a tool", "capability toggle", "perform an operation",
		"fictional_narrative_state", "unrelated crown archive", "meters", "inventory",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("model-visible prompt exposes %q: %s", forbidden, prompt)
		}
	}
}

func TestRoleplayGroundedResponseRejectsUnavailableAndModelAuthoredCitations(t *testing.T) {
	t.Parallel()
	input := roleplayGroundedFixture()
	for _, decision := range []RoleplayGroundedResponseDecision{
		{Schema: RoleplayGroundedResponseSchemaV1, Paragraphs: []RoleplayGroundedParagraph{{
			Text: "The period is about one year.", EvidenceIDs: []string{"missing"},
		}}},
		{Schema: RoleplayGroundedResponseSchemaV1, Paragraphs: []RoleplayGroundedParagraph{{
			Text: "The period is about one year [1].", EvidenceIDs: []string{"doc-1"},
		}}},
	} {
		if err := decision.ValidateFor(input); err == nil {
			t.Fatalf("invalid grounded response accepted: %#v", decision)
		}
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
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
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
