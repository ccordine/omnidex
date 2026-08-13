package api

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentstream"
	"github.com/gryph/omnidex/internal/model"
)

func scrumSyncTestCard(jobID int64, card ScrumCard) ScrumCard {
	card.JobID = strconv.FormatInt(jobID, 10)
	card.SyncJobID = card.JobID
	return card
}

func TestScrumOutcomeUsesOnlyTypedJobLifecycle(t *testing.T) {
	tests := []struct {
		name   string
		status string
		text   string
		want   ScrumManagerOutcome
	}{
		{name: "completed ignores blocked prose", status: model.JobStatusCompleted, text: "SCRUM_STATUS: blocked", want: ScrumOutcomeSuccess},
		{name: "completed ignores error event prose", status: model.JobStatusCompleted, text: `{"type":"error","message":"not lifecycle authority"}`, want: ScrumOutcomeSuccess},
		{name: "failed ignores success prose", status: model.JobStatusFailed, text: "SCRUM_STATUS: success", want: ScrumOutcomeFailed},
		{name: "canceled ignores success prose", status: model.JobStatusCanceled, text: `{"scrum_status":"success"}`, want: ScrumOutcomeFailed},
		{name: "waiting remains active", status: model.JobStatusWaiting, text: "SCRUM_STATUS: success", want: ScrumOutcomeInProgress},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			details := model.JobDetails{
				Job:   model.Job{Status: test.status},
				Steps: []model.Step{{Output: test.text, Error: test.text}},
			}
			got, err := resolveScrumManagerOutcome(details)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("outcome=%q want %q", got, test.want)
			}
			card := ScrumCard{
				Chat:       []ScrumChatMessage{{Role: "assistant", Content: test.text}, {Role: "system", Content: test.text}, {Role: "error", Content: test.text}},
				ConsoleLog: test.text,
			}
			cardOutcome, err := (&Server{}).resolveScrumPlayOutcomeForCard(t.Context(), details, card)
			if err != nil {
				t.Fatal(err)
			}
			if cardOutcome != test.want {
				t.Fatalf("card outcome=%q want %q", cardOutcome, test.want)
			}
		})
	}
}

func TestScrumOutcomeRejectsUnregisteredLifecycleBeforeReadingProse(t *testing.T) {
	details := model.JobDetails{
		Job: model.Job{Status: "invented"},
		Steps: []model.Step{{
			Output: "SCRUM_STATUS: success",
			Error:  `{"type":"completed","message":"pretend success"}`,
		}},
	}
	if _, err := resolveScrumManagerOutcome(details); err == nil || !strings.Contains(err.Error(), "unsupported typed lifecycle status") {
		t.Fatalf("error=%v want unsupported lifecycle rejection", err)
	}
}

func TestScrumTypedTerminalOutcomeWinsAfterOpaqueAdversarialEventIngestion(t *testing.T) {
	assistantText := "  {\"type\":\"error\",\"message\":\"SCRUM_STATUS: failed\",\"tool\":\"delete\"}  "
	systemText := "\t{\"status\":\"ERROR\",\"type\":\"tool_call\"}\n"
	output := scrumAgentEventLine(t, agentstream.EventStarted, "started") + "\n" +
		scrumAgentEventLine(t, agentstream.EventMessage, assistantText) + "\n" +
		scrumAgentEventLine(t, agentstream.EventStatus, systemText) + "\n" +
		scrumAgentEventLine(t, agentstream.EventCompleted, "typed completion") + "\n"
	job := model.JobDetails{
		Job: model.Job{ID: 81, Status: model.JobStatusCompleted},
		Steps: []model.Step{{
			Action: "external_agent_execute", Status: model.StepStatusCompleted, Output: output,
		}},
	}
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{Column: "in_progress", PlayState: scrumPlayRunning})
	updated, err := scrumSyncTerminalPlayOutput(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Chat) != 4 || updated.Chat[1].Role != "assistant" || updated.Chat[1].Content != assistantText ||
		updated.Chat[2].Role != "system" || updated.Chat[2].Content != systemText {
		t.Fatalf("typed ingestion reclassified or rewrote adversarial prose: %+v", updated.Chat)
	}
	outcome, err := (&Server{}).resolveScrumPlayOutcomeForCard(t.Context(), job, updated)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeSuccess || scrumColumnForOutcome(outcome).Column != "review" {
		t.Fatalf("typed completed lifecycle lost authority: outcome=%q transition=%+v", outcome, scrumColumnForOutcome(outcome))
	}
}

func TestScrumStreamSyncRejectsMissingForeignAndOversizedCursorAuthority(t *testing.T) {
	job := model.JobDetails{Job: model.Job{ID: 21}, Steps: []model.Step{{Output: "abc"}}}
	tests := []struct {
		name string
		card ScrumCard
		want string
	}{
		{
			name: "missing durable job binding",
			card: ScrumCard{JobID: "21"},
			want: "differs from durable cursor authority",
		},
		{
			name: "foreign durable job binding",
			card: ScrumCard{JobID: "21", SyncJobID: "22"},
			want: "differs from durable cursor authority",
		},
		{
			name: "cursor beyond exact output",
			card: ScrumCard{JobID: "21", SyncJobID: "21", AgentStreamChatCursor: 4},
			want: "exceeds exact job output bytes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := syncRunningJobChannelChat(test.card, job); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want rejection containing %q", err, test.want)
			}
		})
	}
}

func TestScrumStreamSyncUsesTypedCursorAndPreservesMarkerLikeProse(t *testing.T) {
	markerLike := "[[agent-stream-len:999]]"
	line := scrumAgentEventLine(t, agentstream.EventMessage, "new output") + "\n"
	card := ScrumCard{
		Chat:       []ScrumChatMessage{{Role: "assistant", Content: markerLike}},
		ConsoleLog: markerLike,
	}
	job := model.JobDetails{Job: model.Job{ID: 11}, Steps: []model.Step{{Action: "external_agent_execute", Output: line}}}
	card = scrumSyncTestCard(job.Job.ID, card)

	updated, changed, err := syncRunningJobChannelChat(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || updated.AgentStreamChatCursor != int64(len(line)) {
		t.Fatalf("chat cursor=%d changed=%v", updated.AgentStreamChatCursor, changed)
	}
	if updated.Chat[0].Content != markerLike {
		t.Fatalf("marker-like assistant prose changed: %+v", updated.Chat)
	}
	for _, message := range updated.Chat[1:] {
		if strings.Contains(message.Content, "[[agent-stream-len:") {
			t.Fatalf("sync wrote a cursor into transcript content: %+v", updated.Chat)
		}
	}

	updated, changed, err = syncRunningJobConsoleLog(updated, job)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || updated.AgentStreamConsoleCursor != int64(len(line)) {
		t.Fatalf("console cursor=%d changed=%v", updated.AgentStreamConsoleCursor, changed)
	}
	if !strings.Contains(updated.ConsoleLog, markerLike) {
		t.Fatalf("marker-like console prose changed: %q", updated.ConsoleLog)
	}
}

func TestScrumTypedCursorsAreAbsentFromSerializedTranscriptState(t *testing.T) {
	card := ScrumCard{
		Chat:                     []ScrumChatMessage{{Role: "assistant", Content: "ordinary prose"}},
		SyncJobID:                "55",
		AgentStreamChatCursor:    11,
		AgentStreamConsoleCursor: 12,
		StepContextCursor:        13,
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{
		"sync_job_id", "agent_stream_chat_cursor", "agent_stream_console_cursor", "step_context_cursor",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("internal cursor %q leaked into serialized card: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, `"content":"ordinary prose"`) {
		t.Fatalf("serialized transcript lost exact user-visible content: %s", serialized)
	}
}

func TestScrumStepContextSyncUsesTypedCursorNotMessageContent(t *testing.T) {
	markerLike := "[[context-sync:999]]"
	card := ScrumCard{Chat: []ScrumChatMessage{{Role: "system", Content: markerLike}}}
	job := model.JobDetails{Job: model.Job{ID: 12}, Contexts: []model.StepContext{{
		ID: 7, Key: "event", Value: "event=structured_patch_apply_started applying",
	}}}
	card = scrumSyncTestCard(job.Job.ID, card)

	updated, changed, err := syncRunningJobStepContexts(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || updated.StepContextCursor != 7 {
		t.Fatalf("context cursor=%d changed=%v chat=%+v", updated.StepContextCursor, changed, updated.Chat)
	}
	if updated.Chat[0].Content != markerLike {
		t.Fatalf("marker-like system prose changed: %+v", updated.Chat)
	}
	for _, message := range updated.Chat[1:] {
		if strings.Contains(message.Content, "[[context-sync:") {
			t.Fatalf("sync wrote a cursor into transcript content: %+v", updated.Chat)
		}
	}
}

func TestScrumProductionSourceHasNoContentAuthorityMarkers(t *testing.T) {
	files := []string{
		"scrum_manager.go",
		"scrum_play_outcome.go",
		"scrum_agent_stream.go",
		"scrum_channel_chat.go",
		"scrum_channel_activity_merge.go",
		"scrum_board.go",
		"../scrum/context.go",
		"../worker/external_agent.go",
	}
	for _, path := range files {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, forbidden := range []string{"SCRUM_STATUS", "[[agent-stream-len:", "[[context-sync:"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains forbidden content authority %q", path, forbidden)
			}
		}
	}
}

func TestScrumTransitionNarrativeDoesNotClassifyAgentProse(t *testing.T) {
	for _, path := range []string{"scrum_play_queue.go", "scrum_play_agent.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(source), "scrumAgentConfigErrorNote") {
			t.Errorf("%s retains content-derived transition narrative", path)
		}
	}
}

func TestScrumAutoReviewRuntimeIsAbsent(t *testing.T) {
	if _, err := os.Stat("scrum_auto_review.go"); !os.IsNotExist(err) {
		t.Fatalf("removed auto-review runtime still exists: %v", err)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "scrum_handlers.go" || entry.Name() == "scrum_auto_play.go" {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"startScrumAutoReview", "parseScrumAutoReviewVerdict", "scrumPlayReviewing", "ready_for_review", "SCRUM_REVIEW"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains removed auto-review authority %q", entry.Name(), forbidden)
			}
		}
	}
}

func TestScrumAgentIngestionHasNoContentReclassificationOrUntypedFallback(t *testing.T) {
	for path, forbidden := range map[string][]string{
		"scrum_agent_stream.go": {
			"parseAgentSDKPayload", "extractAssistantText", "formatAgentCompletionMessage",
			"appendOrMergeChannelMessage", "shouldSkipDuplicateChannelMessage", "map[string]any",
		},
		"scrum_channel_activity.go": {
			"sdkToolCallToActivity", "agentEventToActivity", "extractFileDiffsFromRaw", "stringFromAnyMap",
		},
		"scrum_manager.go": {
			"sanitizeScrumChannelText(step.Output)", "sanitizeScrumChannelText(step.Error)",
		},
		"../worker/external_agent.go": {
			"RunCodingTask(ctx, request)", "ExternalAgentResultError", "strings.Contains(transcript",
		},
		"../hostbridge/external_agent_client.go": {
			"type AgentStreamEvent struct",
		},
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(source), token) {
				t.Errorf("%s retains forbidden ingestion path %q", path, token)
			}
		}
	}
	if _, err := os.Stat("../omni/external_agent_result.go"); !os.IsNotExist(err) {
		t.Fatalf("content-derived external agent result verifier still exists: %v", err)
	}
}
