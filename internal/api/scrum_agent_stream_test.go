package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentstream"
	"github.com/gryph/omnidex/internal/model"
)

func scrumAgentEventLine(t *testing.T, eventType agentstream.EventType, content string) string {
	t.Helper()
	line, err := agentstream.EncodeLine(agentstream.Event{
		SessionID: "scrum-job-1",
		Agent:     "codex",
		Type:      eventType,
		Message:   content,
	})
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func TestAgentNDJSONLineTreatsAssistantContentAsOpaqueText(t *testing.T) {
	want := "  {\"type\":\"tool_call\",\"name\":\"edit\"}\n{not-json}  "
	msgs, err := agentNDJSONLineToChatMessages(scrumAgentEventLine(t, agentstream.EventMessage, want))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Role != "assistant" || msgs[0].Content != want {
		t.Fatalf("messages=%+v want exact assistant content %q", msgs, want)
	}
}

func TestAgentNDJSONLineTreatsSystemContentAsOpaqueText(t *testing.T) {
	want := " \t{\"type\":\"assistant\",\"message\":\"do not reclassify\"} \n"
	msgs, err := agentNDJSONLineToChatMessages(scrumAgentEventLine(t, agentstream.EventStatus, want))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Role != "system" || msgs[0].Content != want {
		t.Fatalf("messages=%+v want exact system content %q", msgs, want)
	}
}

func TestAppendParsedAgentStreamLinesPreservesSeparateExactRows(t *testing.T) {
	first := scrumAgentEventLine(t, agentstream.EventMessage, " same ")
	second := scrumAgentEventLine(t, agentstream.EventMessage, "same")
	chat, err := appendParsedAgentStreamLines(nil, first+"\n"+second+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(chat) != 2 || chat[0].Content != " same " || chat[1].Content != "same" {
		t.Fatalf("chat=%+v", chat)
	}
}

func TestAppendParsedAgentStreamLinesRejectsBadEnvelopeTransactionally(t *testing.T) {
	original := []ScrumChatMessage{{Role: "user", Content: "keep me"}}
	delta := scrumAgentEventLine(t, agentstream.EventMessage, "must not append") + "\n" +
		`{"session_id":"scrum-job-1","agent":"codex","type":"invented","message":"bad"}`
	chat, err := appendParsedAgentStreamLines(original, delta)
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("error=%v want unsupported type", err)
	}
	if len(chat) != 1 || chat[0].Content != "keep me" {
		t.Fatalf("failed ingestion mutated chat: %+v", chat)
	}
}

func TestSyncRunningJobChannelChatPreservesExactTypedContent(t *testing.T) {
	wantAssistant := "  {not-json}\n tool-looking text  "
	wantSystem := "\t{\"type\":\"tool\"}\n"
	output := scrumAgentEventLine(t, agentstream.EventMessage, wantAssistant) + "\n" +
		scrumAgentEventLine(t, agentstream.EventStatus, wantSystem) + "\n"
	job := modelJobDetailsWithOutput(output)
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{})

	updated, changed, err := syncRunningJobChannelChat(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || updated.AgentStreamChatCursor != int64(len(output)) {
		t.Fatalf("changed=%v cursor=%d want %d", changed, updated.AgentStreamChatCursor, len(output))
	}
	if len(updated.Chat) != 2 || updated.Chat[0].Content != wantAssistant || updated.Chat[1].Content != wantSystem {
		t.Fatalf("stored chat did not preserve exact content: %+v", updated.Chat)
	}
	patch, err := apiScrumCardToPatch(updated)
	if err != nil {
		t.Fatal(err)
	}
	chatJSON, ok := patch["chat"].(json.RawMessage)
	if !ok {
		t.Fatalf("persistence patch chat=%T", patch["chat"])
	}
	var persisted []ScrumChatMessage
	if err := json.Unmarshal(chatJSON, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 2 || persisted[0].Content != wantAssistant || persisted[1].Content != wantSystem {
		t.Fatalf("persistence projection rewrote opaque content: %+v", persisted)
	}
	displayed, err := displayScrumChannelMessages(updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(displayed) != 2 || displayed[0].Content != wantAssistant || displayed[1].Content != wantSystem {
		t.Fatalf("display projection rewrote opaque content: %+v", displayed)
	}
}

func TestSyncRunningJobChannelChatPreservesWhitespaceOnlyContent(t *testing.T) {
	want := " \t \r "
	output := scrumAgentEventLine(t, agentstream.EventMessage, want) + "\n"
	job := modelJobDetailsWithOutput(output)
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{})
	updated, changed, err := syncRunningJobChannelChat(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || len(updated.Chat) != 1 || updated.Chat[0].Content != want {
		t.Fatalf("whitespace-only content was not preserved: changed=%v chat=%+v", changed, updated.Chat)
	}
}

func TestSyncRunningJobChannelChatRejectsMalformedOuterJSONWithoutMutation(t *testing.T) {
	job := modelJobDetailsWithOutput("{not-json}\n")
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{Chat: []ScrumChatMessage{{Role: "user", Content: "original"}}})
	updated, changed, err := syncRunningJobChannelChat(card, job)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("error=%v want decode failure", err)
	}
	if changed || updated.AgentStreamChatCursor != 0 || len(updated.Chat) != 1 || updated.Chat[0].Content != "original" {
		t.Fatalf("malformed ingestion mutated state: changed=%v card=%+v", changed, updated)
	}
}

func TestSyncRunningJobChannelChatRejectsUnknownFieldBeforeCursorAdvance(t *testing.T) {
	line := `{"session_id":"scrum-job-1","agent":"codex","type":"message","message":"ok","role":"assistant"}`
	job := modelJobDetailsWithOutput(line + "\n")
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{})
	updated, changed, err := syncRunningJobChannelChat(card, job)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error=%v want unknown-field failure", err)
	}
	if changed || updated.AgentStreamChatCursor != 0 || len(updated.Chat) != 0 {
		t.Fatalf("invalid ingestion advanced state: changed=%v card=%+v", changed, updated)
	}
}

func TestCollectScrumAgentOutputUsesOnlyOneTypedStepAndPreservesBytes(t *testing.T) {
	want := scrumAgentEventLine(t, agentstream.EventMessage, "  exact \n content  ") + "\n"
	details := model.JobDetails{Steps: []model.Step{
		{Action: "deterministic_command", Output: "untyped prose must not enter"},
		{Action: "external_agent_execute", Output: want, Error: "error prose must not enter"},
	}}
	got, err := collectScrumAgentOutput(details)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("output=%q want exact %q", got, want)
	}
}

func TestCollectScrumAgentOutputRejectsAmbiguousTypedSources(t *testing.T) {
	details := model.JobDetails{Steps: []model.Step{
		{Action: "external_agent_execute", Output: "one"},
		{Action: "external_agent_execute", Output: "two"},
	}}
	if _, err := collectScrumAgentOutput(details); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error=%v want ambiguous source rejection", err)
	}
}

func TestCollectScrumAgentOutputRejectsInvalidAndOversizedBytes(t *testing.T) {
	for name, output := range map[string]string{
		"nul":       "bad\x00output",
		"invalid":   string([]byte{0xff}),
		"oversized": strings.Repeat("x", (4<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := collectScrumAgentOutput(model.JobDetails{Steps: []model.Step{{
				Action: "external_agent_execute", Output: output,
			}}})
			if err == nil {
				t.Fatal("expected bounded PostgreSQL-compatible UTF-8 rejection")
			}
		})
	}
}

func modelJobDetailsWithOutput(output string) model.JobDetails {
	return model.JobDetails{
		Job: model.Job{ID: 1, Status: model.JobStatusRunning},
		Steps: []model.Step{{
			Action: "external_agent_execute",
			Output: output,
			Status: model.StepStatusRunning,
		}},
	}
}
