package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/roleplay"
)

func TestConversationResponseIsOneBoundedLeaf(t *testing.T) {
	exact := "  Tell me a short story about rain.  \n"
	input := ConversationResponseInput{Kind: ObjectiveKindStory, ExactInstruction: exact}
	job, err := NewConversationResponseJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkConversationResponse || !strings.Contains(prompt, exact) {
		t.Fatalf("job=%#v prompt=%q", job, prompt)
	}
	assertExactObjectSchemaFields(t, schema, []string{"schema", "text"})
	properties := schema["properties"].(map[string]any)
	textSchema := properties["text"].(map[string]any)
	if _, providerHostileBound := textSchema["maxLength"]; providerHostileBound {
		t.Fatalf("conversation response schema contains a provider-hostile grammar repetition: %#v", textSchema)
	}
	assertExactJSONFields(t, reflect.TypeOf(input), []string{
		"kind", "exact_instruction", "objective_context", "roleplay_identity", "roleplay_user_turn",
	})
	assertExactJSONFields(t, reflect.TypeOf(ConversationResponseDecision{}), []string{"schema", "text"})
	for _, forbidden := range []string{"tool", "action", "plan", "memory_write", "completion", "capabilit"} {
		if strings.Contains(strings.ToLower(string(job.Payload)), `"`+forbidden) {
			t.Fatalf("payload exposes forbidden field %q: %s", forbidden, job.Payload)
		}
	}
	for _, forbidden := range []string{"call tools", "manage memory", "choose capabilities", "verify completion"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("conversation response prompt describes unavailable framework capability %q", forbidden)
		}
	}
}

func TestConversationResponseCarriesOnlyIdentityAndMinifiedContextForStory(t *testing.T) {
	relevantContext := "Rain has just begun at the western quay."
	fullSimulationSentinels := []string{
		"omnidex.roleplay-simulation-narrative.v1",
		"The eastern crown opens a hidden observatory.",
		"inventory_brass_key",
		"meter_suspicion_87",
		"candidate_provider_pgvector",
		"search_terms_harbor_archive",
	}
	hiddenSource := strings.Join(fullSimulationSentinels, " | ")
	input := ConversationResponseInput{
		Kind: ObjectiveKindStory, ExactInstruction: "Continue.",
		Context: minifiedObjectiveContext(relevantContext),
		RoleplayIdentity: &RoleplayResponseIdentity{
			CharacterName: "Bob", Summary: "The harbor watchman.", Voice: "Quiet.",
		},
		RoleplayUserTurn: &RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Gryph",
			PersonaSummary:   "An artificer from afar.",
			ContributionKind: roleplay.UserContributionAction,
		},
	}
	input.Context.Capsules[0].Sources[0] = ObjectiveContextSource{
		Namespace:     "roleplay_canon",
		CandidateID:   "CTX_1",
		ContentSHA256: ExactObjectiveContextSHA(hiddenSource),
	}
	prompt, err := BuildConversationResponsePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"character_name":"Bob"`) ||
		!strings.Contains(prompt, relevantContext) ||
		!strings.Contains(prompt, "COMPILED_OBJECTIVE_CONTEXT_JSON") ||
		!strings.Contains(prompt, "The exact user turn controls the immediate response") ||
		!strings.Contains(prompt, "background state supports the response") ||
		!strings.Contains(prompt, "Begin with the character's direct reaction or reply") ||
		!strings.Contains(prompt, `"persona_name":"Gryph"`) ||
		!strings.Contains(prompt, `"contribution_kind":"action"`) ||
		!strings.Contains(prompt, `The responding character is "Bob"; the user-controlled persona is "Gryph"`) ||
		!strings.Contains(prompt, `Never narrate "Gryph"'s action as "Bob"'s action`) ||
		!strings.Contains(prompt, "A short user turn permits a short response") ||
		!strings.Contains(prompt, "no more than 2048 UTF-8 bytes") ||
		!strings.Contains(prompt, "one to three short paragraphs") {
		t.Fatalf("prompt=%s", prompt)
	}
	for _, forbidden := range append(fullSimulationSentinels,
		"ROLEPLAY_CONTEXT_JSON", `"scene"`, `"participants"`, `"meters"`, `"inventory"`,
		`"visible_facts"`, `"memories"`, `"recent_events"`, `"candidate_authorities"`,
		`"query_terms"`, `"provider"`, `"candidate_id"`, `"content_sha256"`,
		ExactObjectiveContextSHA(hiddenSource)) {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("roleplay prompt leaked full simulation or sieve input %q: %s", forbidden, prompt)
		}
	}
	oversized := ConversationResponseDecision{
		Schema: ConversationResponseSchemaV1,
		Text:   strings.Repeat("x", 2049),
	}
	if err := oversized.ValidateFor(input); err == nil {
		t.Fatal("roleplay response accepted prose beyond its live-chat byte budget")
	}
	metadataEcho := ConversationResponseDecision{
		Schema: ConversationResponseSchemaV1,
		Text:   `{"name":"Bob","voice":"Quiet."}`,
	}
	if err := metadataEcho.ValidateFor(input); err == nil {
		t.Fatal("roleplay response accepted copied JSON metadata as narrative prose")
	}
	input.Kind = ObjectiveKindAnswer
	if _, err := NewConversationResponseJob(input); err == nil {
		t.Fatal("answer station accepted fictional character authority")
	}
}

func TestConversationResponseAllowsCharacterWithoutOptionalVoiceDirections(t *testing.T) {
	input := ConversationResponseInput{
		Kind: ObjectiveKindStory, ExactInstruction: "Hello.",
		RoleplayIdentity: &RoleplayResponseIdentity{
			CharacterName: "Ilya", Summary: "A night-shift radio operator.", Voice: "",
		},
		RoleplayUserTurn: &RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionDirection,
		},
	}
	if _, err := NewConversationResponseJob(input); err != nil {
		t.Fatalf("optional empty voice rejected: %v", err)
	}
}

func TestConversationResponseRejectsRoleplayWithoutDistinctUserTurnAuthority(t *testing.T) {
	t.Parallel()

	identity := &RoleplayResponseIdentity{
		CharacterName: "Mara Vey", Summary: "The archive keeper.", Voice: "Dry and direct.",
	}
	if _, err := NewConversationResponseJob(ConversationResponseInput{
		Kind: ObjectiveKindStory, ExactInstruction: "How are you?", RoleplayIdentity: identity,
	}); err == nil {
		t.Fatal("roleplay response accepted an untyped user speaker and contribution")
	}
	if _, err := NewConversationResponseJob(ConversationResponseInput{
		Kind: ObjectiveKindStory, ExactInstruction: "How are you?",
		RoleplayUserTurn: &RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaCharacter, PersonaName: "Gryph",
			PersonaSummary: "An artificer.", ContributionKind: roleplay.UserContributionDialogue,
		},
	}); err == nil {
		t.Fatal("roleplay user authority was accepted without one responding character")
	}
}

func TestConversationResponseProjectsOnlyMinifiedContextCapsule(t *testing.T) {
	t.Parallel()
	input := ConversationResponseInput{
		Kind: ObjectiveKindAnswer, ExactInstruction: "Which one?",
		Context: minifiedObjectiveContext("The prior answer recommended the write-through cache."),
	}
	prompt, err := BuildConversationResponsePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "recommended the write-through") {
		t.Fatalf("minified context capsule missing from response prompt: %s", prompt)
	}
	for _, forbidden := range []string{
		"user_authorities", "assistant_results", `"message_id"`, `"job_id"`,
		`"candidate_id"`, `"content_sha256"`, `"namespace"`,
		`"retrieval_concepts"`, `"search_terms"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("raw context field %q leaked into response prompt: %s", forbidden, prompt)
		}
	}
}

func TestConversationResponseRejectsUnsupportedKindAndInexactJSON(t *testing.T) {
	if _, err := NewConversationResponseJob(ConversationResponseInput{
		Kind: ObjectiveKindExternalAnswer, ExactInstruction: "What changed?",
	}); err == nil {
		t.Fatal("external answer bypassed grounded evidence workflow")
	}
	input := ConversationResponseInput{Kind: ObjectiveKindAnswer, ExactInstruction: "Hello"}
	for _, raw := range []string{
		`{"schema":"omnidex.conversation-response.v1","text":"Hi","action":"reply"}`,
		`{"schema":"omnidex.conversation-response.v1","text":"Hi","text":"Again"}`,
		`{"Schema":"omnidex.conversation-response.v1","text":"Hi"}`,
		`{"schema":"omnidex.conversation-response.v1","text":" Hi "}`,
		strings.Repeat("x", maxPortableCandidateBytes+1),
	} {
		if _, err := DecodeConversationResponseDecision(input, raw); err == nil {
			t.Fatalf("invalid response accepted: %.80q", raw)
		}
	}
}
