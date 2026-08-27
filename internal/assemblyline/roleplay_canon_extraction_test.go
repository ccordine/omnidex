package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCanonExtractionReturnsOnlyNewBoundedFacts(t *testing.T) {
	antecedent := RoleplayCanonAntecedent{
		PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Gryph",
		ContributionKind:    roleplay.UserContributionDialogue,
		ContributionContext: "I hand Bob the silver key.",
	}
	input := RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind: RoleplayCanonSourceAssistantResponse, AttributedPersonaName: "Bob",
			ExactContribution: "Rain began over the harbor as Bob closed the west gate.",
		},
		AntecedentUserTurn: &antecedent,
		Context:            minifiedObjectiveContext("Bob is at the harbor."),
	}
	job, err := NewRoleplayCanonExtractionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.Source.ExactContribution) ||
		!strings.Contains(prompt, "exactly one accepted contribution") ||
		!strings.Contains(prompt, "use it only to resolve references") ||
		!strings.Contains(prompt, `"persona_name":"Gryph"`) ||
		!strings.Contains(prompt, `only to "Bob"`) ||
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

func TestRoleplayCanonExtractionRejectsDuplicateCandidates(t *testing.T) {
	antecedent := RoleplayCanonAntecedent{
		PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Gryph",
		ContributionKind:    roleplay.UserContributionDialogue,
		ContributionContext: "Hello.",
	}
	input := RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind: RoleplayCanonSourceAssistantResponse, AttributedPersonaName: "Mara",
			ExactContribution: "Mara closes the notebook and looks up from the astrolabe.",
		},
		AntecedentUserTurn: &antecedent,
		Context:            minifiedObjectiveContext("The astrolabe ticks backward."),
	}
	_, err := DecodeRoleplayCanonExtractionDecision(input,
		`{"schema":"omnidex.roleplay-canon-extraction.v1","facts":["Mara closes the notebook.","Mara closes the notebook.","The astrolabe ticks backward."]}`,
	)
	if err == nil {
		t.Fatal("duplicate canon candidates were silently accepted")
	}

	decision, err := DecodeRoleplayCanonExtractionDecision(input,
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
	antecedent := RoleplayCanonAntecedent{
		PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Gryph",
		ContributionKind:    roleplay.UserContributionAction,
		ContributionContext: "I greet Bob.",
	}
	input := RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind: RoleplayCanonSourceAssistantResponse, AttributedPersonaName: "Bob",
			ExactContribution: "Bob nods once in reply.",
		},
		AntecedentUserTurn: &antecedent,
		Context:            minifiedObjectiveContext("Bob is the harbor watchman."),
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
		"source", "antecedent_user_turn", "context",
	})
	assertExactJSONFields(t, reflect.TypeOf(input.Source), []string{
		"kind", "attributed_persona_name", "exact_contribution", "persona_kind", "contribution_kind",
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

func TestRoleplayCanonExtractionHasExactlyOneFactSource(t *testing.T) {
	userTurn := roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind: roleplay.UserContributionNarration,
		Parts:            []roleplay.UserTurnPart{{Kind: roleplay.UserTurnPartEvent, Text: "The bronze bell cracks."}},
		ExactText:        "[Event]\nThe bronze bell cracks.",
	}
	userSource, err := ProjectRoleplayUserCanonSource(userTurn)
	if err != nil {
		t.Fatal(err)
	}
	userPrompt, err := BuildRoleplayCanonExtractionPrompt(RoleplayCanonExtractionInput{
		Source: userSource, Context: ObjectiveContext{Capsules: []ObjectiveContextCapsule{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(userPrompt, "The bronze bell cracks.") ||
		strings.Contains(userPrompt, "ASSISTANT_SOURCE_SENTINEL") ||
		strings.Contains(userPrompt, `"antecedent_user_turn":`) ||
		strings.Contains(userPrompt, "persona_summary") ||
		strings.Contains(userPrompt, `"parts":`) {
		t.Fatalf("user-source prompt crossed source authority: %s", userPrompt)
	}

	antecedent, err := ProjectRoleplayCanonAntecedent(userTurn, userTurn.ExactText)
	if err != nil {
		t.Fatal(err)
	}
	assistantSource, err := NewRoleplayAssistantCanonSource(
		"Mara", "I agree that the bell is broken.",
	)
	if err != nil {
		t.Fatal(err)
	}
	assistantPrompt, err := BuildRoleplayCanonExtractionPrompt(RoleplayCanonExtractionInput{
		Source: assistantSource, AntecedentUserTurn: &antecedent,
		Context: ObjectiveContext{Capsules: []ObjectiveContextCapsule{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(assistantPrompt, "I agree that the bell is broken.") ||
		!strings.Contains(assistantPrompt, "Never extract a fact from the antecedent user turn") ||
		strings.Contains(assistantPrompt, `"user_turn":{"`) {
		t.Fatalf("assistant-source prompt crossed source authority: %s", assistantPrompt)
	}
	if _, err := NewRoleplayCanonExtractionJob(RoleplayCanonExtractionInput{
		Source:  assistantSource,
		Context: ObjectiveContext{Capsules: []ObjectiveContextCapsule{}},
	}); err == nil {
		t.Fatal("assistant canon source without typed antecedent was accepted")
	}
}

func TestRoleplayCanonExtractionEnforcesSourceSpecificByteBounds(t *testing.T) {
	emptyContext := ObjectiveContext{Capsules: []ObjectiveContextCapsule{}}
	validAntecedent := RoleplayCanonAntecedent{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind:    roleplay.UserContributionNarration,
		ContributionContext: "The bell cracked.",
	}
	for _, input := range []RoleplayCanonExtractionInput{
		{
			Source: RoleplayCanonSource{
				Kind:                  RoleplayCanonSourceUserContribution,
				AttributedPersonaName: roleplay.NarratorPersonaName,
				ExactContribution:     strings.Repeat("u", roleplay.MaxUserTurnBytes+1),
				PersonaKind:           roleplay.UserPersonaNarrator,
				ContributionKind:      roleplay.UserContributionNarration,
			},
			Context: emptyContext,
		},
		{
			Source: RoleplayCanonSource{
				Kind:                  RoleplayCanonSourceAssistantResponse,
				AttributedPersonaName: "Mara",
				ExactContribution: strings.Repeat(
					"a", roleplay.MaxNarrativeResponseBytes+1,
				),
			},
			AntecedentUserTurn: &validAntecedent,
			Context:            emptyContext,
		},
		{
			Source: RoleplayCanonSource{
				Kind:                  RoleplayCanonSourceAssistantResponse,
				AttributedPersonaName: "Mara", ExactContribution: "Mara nods.",
			},
			AntecedentUserTurn: &RoleplayCanonAntecedent{
				PersonaKind:      roleplay.UserPersonaNarrator,
				PersonaName:      roleplay.NarratorPersonaName,
				ContributionKind: roleplay.UserContributionNarration,
				ContributionContext: strings.Repeat(
					"u", roleplay.MaxUserTurnBytes+1,
				),
			},
			Context: emptyContext,
		},
	} {
		if _, err := NewRoleplayCanonExtractionJob(input); err == nil {
			t.Fatalf("over-bound roleplay canon envelope accepted: %#v", input)
		}
	}
}

func TestRoleplayUserCanonFinalBoundaryRejectsMislabeledRawCommandBytes(t *testing.T) {
	input := RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind:                  RoleplayCanonSourceUserContribution,
			AttributedPersonaName: "Mara",
			ExactContribution:     "/research the private archive",
			PersonaKind:           roleplay.UserPersonaCharacter,
			ContributionKind:      roleplay.UserContributionDialogue,
		},
		Context: ObjectiveContext{Capsules: []ObjectiveContextCapsule{}},
	}
	if _, err := NewRoleplayCanonExtractionJob(input); err == nil ||
		!strings.Contains(err.Error(), "raw command bytes") {
		t.Fatalf("mislabeled raw command job error=%v", err)
	}
	if prompt, err := BuildRoleplayCanonExtractionPrompt(input); err == nil ||
		prompt != "" || !strings.Contains(err.Error(), "raw command bytes") {
		t.Fatalf("mislabeled raw command prompt=%q error=%v", prompt, err)
	}
	input = RoleplayCanonExtractionInput{
		Source: RoleplayCanonSource{
			Kind:                  RoleplayCanonSourceAssistantResponse,
			AttributedPersonaName: "Ivo", ExactContribution: "Ivo nods.",
		},
		AntecedentUserTurn: &RoleplayCanonAntecedent{
			PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Mara",
			ContributionKind:    roleplay.UserContributionDialogue,
			ContributionContext: "/research the private archive",
		},
		Context: ObjectiveContext{Capsules: []ObjectiveContextCapsule{}},
	}
	if prompt, err := BuildRoleplayCanonExtractionPrompt(input); err == nil ||
		prompt != "" || !strings.Contains(err.Error(), "raw command bytes") {
		t.Fatalf("mislabeled raw command antecedent prompt=%q error=%v", prompt, err)
	}
}
