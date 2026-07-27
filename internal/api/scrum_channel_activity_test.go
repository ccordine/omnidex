package api

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestSyncRunningJobStepContexts(t *testing.T) {
	card := ScrumCard{Chat: []ScrumChatMessage{{Role: "system", Content: "Job queued"}}}
	job := model.JobDetails{
		Contexts: []model.StepContext{
			{ID: 1, Key: "event", Value: "time=2026-05-29T10:00:00Z event=structured_patch_apply_started Applying structured patch artifact"},
			{ID: 2, Key: "event", Value: "time=2026-05-29T10:00:01Z event=structured_patch_apply_finished files=2"},
		},
	}
	updated, ok := syncRunningJobStepContexts(card, job)
	if !ok {
		t.Fatal("expected context sync")
	}
	toolCount := 0
	for _, msg := range updated.Chat {
		if msg.Role == "tool" {
			toolCount++
		}
	}
	if toolCount < 2 {
		t.Fatalf("chat=%+v", updated.Chat)
	}
}

func TestStepContextCommandOutput(t *testing.T) {
	msgs := stepContextToActivity(model.StepContext{
		ID:    3,
		Key:   "tool_stdout",
		Value: "@@ -1,3 +1,3 @@\n-old\n+new",
	})
	if len(msgs) != 1 {
		t.Fatalf("messages=%+v", msgs)
	}
	activity, ok := parseChannelActivity(msgs[0].Content)
	if !ok || activity.Activity != "file_change" {
		t.Fatalf("activity=%+v", activity)
	}
}

func TestRemovedWebSearchDegradedEventIsNotSilentlyHidden(t *testing.T) {
	if isNoisyStepEvent("web_search_degraded") {
		t.Fatal("removed web_search_degraded event must not remain a hidden compatibility path")
	}
}

func TestCommandActivityUsesConciseTitleAndKeepsFullCommandInDetails(t *testing.T) {
	command := "go test ./...\nGOCACHE=/tmp/omni-cache another very long command segment that should not fill the activity screen"
	message := commandActivity(command, "running", "")
	activity, ok := parseChannelActivity(message.Content)
	if !ok {
		t.Fatalf("activity not encoded: %s", message.Content)
	}
	if activity.Command != command {
		t.Fatalf("command details were lost: %q", activity.Command)
	}
	if len(activity.Title) > 96 || activity.Title == command {
		t.Fatalf("title is not concise: %q", activity.Title)
	}
}

func TestCollapseScrumChannelDisplayMessagesReplacesRunningToolWithCompletion(t *testing.T) {
	running := commandActivity("go test ./internal/api", "running", "starting")
	running.CreatedAt = "2026-07-26T12:00:00Z"
	completed := commandActivity("go test ./internal/api", "completed", "all tests passed")
	completed.CreatedAt = "2026-07-26T12:00:01Z"

	collapsed := collapseScrumChannelDisplayMessages([]ScrumChatMessage{running, completed})
	if len(collapsed) != 1 {
		t.Fatalf("messages=%+v want one lifecycle row", collapsed)
	}
	activity, ok := parseChannelActivity(collapsed[0].Content)
	if !ok || activity.Status != "completed" || activity.Detail != "all tests passed" {
		t.Fatalf("activity=%+v", activity)
	}
}

func TestCollapseScrumChannelDisplayMessagesMergesToolLifecycleSuffixes(t *testing.T) {
	running := toolCallActivity("tool_call_started", "", "running", "starting")
	running.CreatedAt = "2026-07-26T12:00:00Z"
	completed := toolCallActivity("tool_call_finished", "", "completed", "finished")
	completed.CreatedAt = "2026-07-26T12:00:01Z"

	collapsed := collapseScrumChannelDisplayMessages([]ScrumChatMessage{running, completed})
	if len(collapsed) != 1 {
		t.Fatalf("messages=%+v want one tool lifecycle row", collapsed)
	}
	activity, ok := parseChannelActivity(collapsed[0].Content)
	if !ok || activity.Status != "completed" || activity.Detail != "finished" {
		t.Fatalf("activity=%+v", activity)
	}
}

func TestCollapseScrumChannelDisplayMessagesCombinesConsecutiveOutput(t *testing.T) {
	first := outputActivity("stdout", "first line")
	second := outputActivity("stdout", "second line")

	collapsed := collapseScrumChannelDisplayMessages([]ScrumChatMessage{first, second})
	if len(collapsed) != 1 {
		t.Fatalf("messages=%+v want one output row", collapsed)
	}
	activity, ok := parseChannelActivity(collapsed[0].Content)
	if !ok || activity.Detail != "first line\nsecond line" {
		t.Fatalf("activity=%+v", activity)
	}
}
