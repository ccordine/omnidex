package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayGroundedResponseReceivesStateAndEvidenceWithoutControlPlane(t *testing.T) {
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
	assertExactJSONFields(t, reflect.TypeOf(input), []string{
		"exact_question", "fictional_narrative_state", "real_world_evidence",
	})
	lower := strings.ToLower(prompt)
	for _, forbidden := range []string{
		"/research", "external_command", "web_research", "resolver", "catalog", "tool schema",
		"call a tool", "choose a tool", "capability toggle", "perform an operation",
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
	return RoleplayGroundedResponseInput{
		ExactQuestion: "What is Earth's orbital period?",
		FictionalNarrativeState: roleplay.NarrativeSimulationProjection{
			Schema: roleplay.NarrativeSimulationProjectionSchemaV1,
			Scene: roleplay.NarrativeScene{
				Title: "Observatory", Description: "A quiet dome beneath the stars.", ActiveCharacterName: "Ada",
			},
			Participants: []string{"Ada"},
			Viewpoint: roleplay.NarrativePersona{
				Name: "Ada", Summary: "A careful astronomer.", Voice: "Measured",
				Traits: []string{"Curious"}, Goals: []string{"Explain clearly"},
			},
			Meters:       []roleplay.NarrativeMeter{{Name: "Focus", Minimum: 0, Maximum: 10, Value: 8}},
			VisibleFacts: []string{"The observatory is open."},
		},
		RealWorldEvidence: []GroundedEvidenceCapsule{{
			ID: "doc-1", Text: "Earth's orbital period is approximately 365.25 days.",
		}},
	}
}
