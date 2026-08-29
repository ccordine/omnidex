package api

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestSyncRunningJobStepContexts(t *testing.T) {
	job := model.JobDetails{
		Job: model.Job{ID: 2, Status: model.JobStatusRunning},
		Contexts: []model.StepContext{
			{ID: 1, Key: "event", Value: "time=2026-05-29T10:00:00Z event=repository_snapshot_started authority=server"},
			{ID: 2, Key: "event", Value: "time=2026-05-29T10:00:01Z event=repository_snapshot_ready snapshot=sha256:abc files=2"},
		},
	}
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{Chat: []ScrumChatMessage{{Role: "system", Content: "Job queued"}}})
	updated, ok, err := syncRunningJobStepContexts(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected context sync")
	}
	toolCount := 0
	for _, msg := range updated.PendingChannelMessages {
		if msg.Role == "tool" {
			toolCount++
		}
	}
	if toolCount != 2 {
		t.Fatalf("pending messages=%+v", updated.PendingChannelMessages)
	}
	if updated.StepContextCursor != 2 {
		t.Fatalf("typed context cursor=%d want 2", updated.StepContextCursor)
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

func TestOnlyCurrentRedundantEventsAreHidden(t *testing.T) {
	if !isNoisyStepEvent("coding_portable_dispatched") {
		t.Fatal("current redundant dispatch event should be hidden")
	}
	if isNoisyStepEvent("plan_begin") {
		t.Fatal("removed plan event must not remain a hidden compatibility path")
	}
}
