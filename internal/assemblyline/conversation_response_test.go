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
	assertExactJSONFields(t, reflect.TypeOf(input), []string{"kind", "exact_instruction", "objective_context", "roleplay_context"})
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

func TestConversationResponseCarriesCharacterKnowledgeOnlyForStory(t *testing.T) {
	projection := &roleplay.NarrativeSimulationProjection{
		Schema: roleplay.NarrativeSimulationProjectionSchemaV1,
		Scene: roleplay.NarrativeScene{
			Title: "Harbor", Description: "Rain falls over the western quay.", ActiveCharacterName: "Bob",
		},
		Participants: []string{"Bob"},
		Viewpoint: roleplay.NarrativePersona{
			Name: "Bob", Summary: "The harbor watchman.", Voice: "Quiet.",
			Traits: []string{}, Goals: []string{},
		},
		Meters: []roleplay.NarrativeMeter{}, Inventory: []roleplay.NarrativeInventoryItem{},
		VisibleFacts: []string{"Rain began over the harbor."},
		Memories:     []string{}, RecentEvents: []string{},
	}
	input := ConversationResponseInput{
		Kind: ObjectiveKindStory, ExactInstruction: "Continue.", RoleplayContext: projection,
	}
	prompt, err := BuildConversationResponsePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"schema":"omnidex.roleplay-simulation-narrative.v1"`) ||
		!strings.Contains(prompt, "Rain began over the harbor.") {
		t.Fatalf("prompt=%s", prompt)
	}
	input.Kind = ObjectiveKindAnswer
	if _, err := NewConversationResponseJob(input); err == nil {
		t.Fatal("answer station accepted fictional character authority")
	}
}

func TestConversationResponseProjectsOnlyCodeSelectedUserAuthorities(t *testing.T) {
	t.Parallel()
	input := ConversationResponseInput{
		Kind: ObjectiveKindAnswer, ExactInstruction: "Which one?",
		Context: ObjectiveContext{
			UserAuthorities: []ConversationSelectedUserAuthority{{
				MessageID: 21, Content: "Compare the read-through and write-through cache.",
			}},
			AssistantResults: []ConversationSelectedAssistantResult{{
				UserMessageID: 21, MessageID: 22, JobID: 10, Content: "I recommended the write-through cache.",
			}},
		},
	}
	prompt, err := BuildConversationResponsePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"message_id":21`) || !strings.Contains(prompt, "read-through") ||
		!strings.Contains(prompt, "recommended the write-through") {
		t.Fatalf("selected authority missing from response prompt: %s", prompt)
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
