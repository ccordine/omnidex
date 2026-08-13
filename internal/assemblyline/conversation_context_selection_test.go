package assemblyline

import (
	"strings"
	"testing"
)

func TestConversationContextSelectionReturnsOnlyPriorUserMessageIDs(t *testing.T) {
	t.Parallel()
	input := conversationContextSelectionFixture()
	job, err := NewConversationContextSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkConversationContextSelection {
		t.Fatalf("job kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"message_id":11`) ||
		!strings.Contains(prompt, `"paired_user_message_id":11`) ||
		!strings.Contains(prompt, `"max_selected_bytes":2048`) ||
		!strings.Contains(prompt, input.ExactInstruction) ||
		schema["additionalProperties"] != false {
		t.Fatalf("prompt/schema lost exact authority: %q %#v", prompt, schema)
	}
	decision := ConversationContextSelectionDecision{
		Schema: ConversationContextSelectionSchemaV1, ReferencedUserMessageIDs: []int64{11},
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]int64{{12}, {11, 11}, {99}} {
		decision.ReferencedUserMessageIDs = invalid
		if err := decision.ValidateFor(input); err == nil {
			t.Fatalf("accepted invalid references %v", invalid)
		}
	}
}

func TestConversationContextSelectionRejectsUnboundedOrMalformedHistory(t *testing.T) {
	t.Parallel()
	input := conversationContextSelectionFixture()
	input.CandidateAuthorities[1].MessageID = input.CandidateAuthorities[0].MessageID
	if _, err := NewConversationContextSelectionJob(input); err == nil {
		t.Fatal("unordered message authority was accepted")
	}
	input = conversationContextSelectionFixture()
	input.CandidateAuthorities[0].Content = strings.Repeat("x", MaxConversationContextCandidateBytes+1)
	if _, err := NewConversationContextSelectionJob(input); err == nil {
		t.Fatal("oversized history was accepted")
	}
	input = conversationContextSelectionFixture()
	input.CandidateAuthorities[0].Role = "tool"
	if _, err := NewConversationContextSelectionJob(input); err == nil {
		t.Fatal("unknown role was accepted")
	}
	input = conversationContextSelectionFixture()
	input.CandidateAuthorities[1].PairedUserMessageID = 99
	if _, err := NewConversationContextSelectionJob(input); err == nil {
		t.Fatal("assistant result with an unavailable user pairing was accepted")
	}
	input = conversationContextSelectionFixture()
	input.MaxSelectedBytes--
	if _, err := NewConversationContextSelectionJob(input); err == nil {
		t.Fatal("caller-defined projection budget was accepted")
	}
}

func TestConversationContextSelectionRejectsUnavailableAndOversizedSelections(t *testing.T) {
	t.Parallel()
	input := conversationContextSelectionFixture()
	for _, ids := range [][]int64{
		{13},
		{11, 11},
		{11, 13, 14, 15, 16, 17, 18, 19, 20},
	} {
		decision := ConversationContextSelectionDecision{
			Schema: ConversationContextSelectionSchemaV1, ReferencedUserMessageIDs: ids,
		}
		if err := decision.ValidateFor(input); err == nil {
			t.Fatalf("invalid selected IDs accepted: %v", ids)
		}
	}
}

func TestConversationContextSelectionEnforcesExactProjectionBudget(t *testing.T) {
	t.Parallel()
	input := ConversationContextSelectionInput{
		ExactInstruction: "Use that result.",
		MaxSelectedBytes: MaxSelectedConversationProjectionBytes,
		CandidateAuthorities: []ConversationContextTurn{
			{MessageID: 21, Role: ConversationContextUser, Content: strings.Repeat("u", 1024)},
			{
				MessageID: 22, Role: ConversationContextAssistant, PairedUserMessageID: 21,
				Content: strings.Repeat("a", 1024),
			},
		},
	}
	decision := ConversationContextSelectionDecision{
		Schema: ConversationContextSelectionSchemaV1, ReferencedUserMessageIDs: []int64{21},
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatalf("exact-bound selection rejected: %v", err)
	}
	input.CandidateAuthorities[1].Content += "x"
	if err := decision.ValidateFor(input); err == nil {
		t.Fatal("selection exceeding its declared projection budget was accepted")
	}
}

func TestConversationContextSelectionDecodesOneClosedIDOnlyResponse(t *testing.T) {
	t.Parallel()
	input := conversationContextSelectionFixture()
	decision, err := DecodeConversationContextSelectionDecision(input,
		`{"schema":"omnidex.conversation-context-selection.v1","referenced_user_message_ids":[11]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.ReferencedUserMessageIDs) != 1 || decision.ReferencedUserMessageIDs[0] != 11 {
		t.Fatalf("decision=%#v", decision)
	}
	if _, err := DecodeConversationContextSelectionDecision(input,
		`{"schema":"omnidex.conversation-context-selection.v1","referenced_user_message_ids":[11],"memory":"invented"}`); err == nil {
		t.Fatal("unknown memory authority was accepted")
	}
}

func conversationContextSelectionFixture() ConversationContextSelectionInput {
	return ConversationContextSelectionInput{
		ExactInstruction: "Which one should I change?",
		MaxSelectedBytes: MaxSelectedConversationProjectionBytes,
		CandidateAuthorities: []ConversationContextTurn{
			{MessageID: 11, Role: ConversationContextUser, Content: "Compare the two cache implementations."},
			{
				MessageID: 12, Role: ConversationContextAssistant, PairedUserMessageID: 11,
				Content: "The first is simpler; the second is faster.",
			},
		},
	}
}
