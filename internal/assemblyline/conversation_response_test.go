package assemblyline

import (
	"reflect"
	"strings"
	"testing"
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
	assertExactJSONFields(t, reflect.TypeOf(input), []string{"kind", "exact_instruction", "objective_context"})
	assertExactJSONFields(t, reflect.TypeOf(ConversationResponseDecision{}), []string{"schema", "text"})
	for _, forbidden := range []string{"tool", "action", "plan", "memory_write", "completion", "capabilit"} {
		if strings.Contains(strings.ToLower(string(job.Payload)), `"`+forbidden) {
			t.Fatalf("payload exposes forbidden field %q: %s", forbidden, job.Payload)
		}
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
