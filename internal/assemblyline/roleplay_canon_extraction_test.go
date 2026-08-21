package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCanonExtractionReturnsOnlyNewBoundedFacts(t *testing.T) {
	input := RoleplayCanonExtractionInput{
		ExactInstruction:        "I hand Bob the silver key and tell him it opens the western archive.",
		AssistantResponse:       "Rain began over the harbor as Bob closed the west gate.",
		RespondingCharacterName: "Bob",
		Context:                 minifiedObjectiveContext("Bob is at the harbor."),
		UserTurn: RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Gryph",
			PersonaSummary: "An artificer.", ContributionKind: roleplay.UserContributionActionDialogue,
		},
	}
	job, err := NewRoleplayCanonExtractionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.AssistantResponse) ||
		!strings.Contains(prompt, "complete accepted turn") ||
		!strings.Contains(prompt, "either exact_instruction or assistant_response") ||
		!strings.Contains(prompt, `"persona_name":"Gryph"`) ||
		!strings.Contains(prompt, `Attribute the exact user contribution to "Gryph"`) ||
		!strings.Contains(prompt, "zero to eight") ||
		!strings.Contains(prompt, "empty fact array") || schema == nil {
		t.Fatalf("prompt=%q schema=%#v", prompt, schema)
	}
	factSchema := schema["properties"].(map[string]any)["facts"].(map[string]any)
	if factSchema["minItems"] != 0 || factSchema["uniqueItems"] != true {
		t.Fatalf("canon schema does not permit one deduplicated zero-delta result: %#v", factSchema)
	}
	decision, err := DecodeRoleplayCanonExtractionDecision(input,
		`{"schema":"omnidex.roleplay-canon-extraction.v1","facts":["Rain began over the harbor.","Bob closed the west gate."]}`,
	)
	if err != nil || len(decision.Facts) != 2 {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
	for _, valid := range []RoleplayCanonExtractionDecision{
		{Schema: RoleplayCanonExtractionSchemaV1, Facts: []string{}},
	} {
		if err := valid.ValidateFor(input); err != nil {
			t.Fatalf("valid zero-delta extraction rejected: %#v: %v", valid, err)
		}
	}
	for _, invalid := range []RoleplayCanonExtractionDecision{
		{Schema: RoleplayCanonExtractionSchemaV1, Facts: nil},
		{Schema: RoleplayCanonExtractionSchemaV1, Facts: []string{"Same.", "Same."}},
	} {
		if err := invalid.ValidateFor(input); err == nil {
			t.Fatalf("invalid extraction accepted: %#v", invalid)
		}
	}
}

func TestRoleplayCanonExtractionDeterministicallyRemovesDuplicateCandidates(t *testing.T) {
	input := RoleplayCanonExtractionInput{
		ExactInstruction:        "Hello",
		AssistantResponse:       "Mara closes the notebook and looks up from the astrolabe.",
		RespondingCharacterName: "Mara",
		Context:                 minifiedObjectiveContext("The astrolabe ticks backward."),
		UserTurn: RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Gryph",
			PersonaSummary: "An artificer.", ContributionKind: roleplay.UserContributionDialogue,
		},
	}
	decision, err := DecodeRoleplayCanonExtractionDecision(input,
		`{"schema":"omnidex.roleplay-canon-extraction.v1","facts":["Mara closes the notebook.","Mara closes the notebook.","The astrolabe ticks backward."]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Facts) != 2 || decision.Facts[0] != "Mara closes the notebook." ||
		decision.Facts[1] != "The astrolabe ticks backward." {
		t.Fatalf("duplicate candidates were not reduced deterministically: %#v", decision)
	}

	decision, err = DecodeRoleplayCanonExtractionDecision(input,
		`{"schema":"omnidex.roleplay-canon-extraction.v1","facts":["Mara closes the notebook.","Mara greets the visitor."]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Facts) != 2 || decision.Facts[0] != "Mara closes the notebook." ||
		decision.Facts[1] != "Mara greets the visitor." {
		t.Fatalf("unique fact projection=%#v", decision)
	}
}

func TestRoleplayCanonExtractionPromptCannotReceiveFullSimulationOrRetrievalInputs(t *testing.T) {
	forbidden := []string{
		"omnidex.roleplay-simulation-narrative.v1",
		"The eastern crown opens a hidden observatory.",
		`"known_facts"`, `"scene"`, `"participants"`, `"meters"`, `"inventory"`,
		`"memories"`, `"recent_events"`, `"candidate_authorities"`, `"query_terms"`,
		"candidate_provider_pgvector",
	}
	hiddenSource := strings.Join(forbidden, " | ")
	input := RoleplayCanonExtractionInput{
		ExactInstruction:        "I greet Bob.",
		AssistantResponse:       "Bob nods once in reply.",
		RespondingCharacterName: "Bob",
		Context:                 minifiedObjectiveContext("Bob is the harbor watchman."),
		UserTurn: RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Gryph",
			PersonaSummary: "An artificer.", ContributionKind: roleplay.UserContributionAction,
		},
	}
	input.Context.Capsules[0].Sources[0] = ObjectiveContextSource{
		Namespace:     "roleplay_canon",
		CandidateID:   "CTX_1",
		ContentSHA256: ExactObjectiveContextSHA(hiddenSource),
	}
	prompt, err := BuildRoleplayCanonExtractionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Bob is the harbor watchman.") {
		t.Fatalf("minified context missing from canon prompt: %s", prompt)
	}
	assertExactJSONFields(t, reflect.TypeOf(input), []string{
		"exact_instruction", "assistant_response", "responding_character_name", "context", "user_turn",
	})
	for _, value := range forbidden {
		if strings.Contains(prompt, value) {
			t.Fatalf("canon prompt leaked full simulation or sieve input %q: %s", value, prompt)
		}
	}
	for _, value := range []string{
		ExactObjectiveContextSHA(hiddenSource), `"candidate_id"`, `"content_sha256"`, `"namespace"`,
	} {
		if strings.Contains(prompt, value) {
			t.Fatalf("canon prompt leaked code-only context authority %q: %s", value, prompt)
		}
	}
}
