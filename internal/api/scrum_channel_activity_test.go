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
