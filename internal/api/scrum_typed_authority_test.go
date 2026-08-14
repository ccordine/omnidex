package api

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

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
				Chat: []ScrumChatMessage{{Role: "assistant", Content: test.text}, {Role: "system", Content: test.text}, {Role: "error", Content: test.text}},
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

func TestScrumStepContextSyncRejectsMissingForeignAndInvalidCursorAuthority(t *testing.T) {
	job := model.JobDetails{Job: model.Job{ID: 21}, Contexts: []model.StepContext{{ID: 1, Key: "event", Value: "event=patch_apply_started"}}}
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
			name: "negative typed cursor",
			card: ScrumCard{JobID: "21", SyncJobID: "21", StepContextCursor: -1},
			want: "step-context cursor must be non-negative",
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

func TestScrumTypedCursorsAreAbsentFromSerializedTranscriptState(t *testing.T) {
	card := ScrumCard{
		Chat:              []ScrumChatMessage{{Role: "assistant", Content: "ordinary prose"}},
		SyncJobID:         "55",
		StepContextCursor: 13,
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{
		"sync_job_id", "step_context_cursor",
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
	if card.Chat[0].Content != markerLike {
		t.Fatalf("marker-like system prose changed: %+v", card.Chat)
	}
	for _, message := range updated.PendingChannelMessages {
		if strings.Contains(message.Content, "[[context-sync:") {
			t.Fatalf("sync wrote a cursor into transcript content: %+v", updated.PendingChannelMessages)
		}
	}
}

func TestScrumProductionSourceHasNoContentAuthorityMarkers(t *testing.T) {
	files := []string{
		"scrum_manager.go",
		"scrum_play_outcome.go",
		"scrum_channel_chat.go",
		"scrum_channel_activity_merge.go",
		"scrum_board.go",
		"../scrum/context.go",
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
	for _, path := range []string{"scrum_agent_stream.go", "scrum_external_agent_outcome.go", "../worker/external_agent.go"} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired content-ingestion runtime remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect retired content-ingestion runtime %s: %v", path, err)
		}
	}
}

func TestScrumTransitionNarrativeDoesNotClassifyAgentProse(t *testing.T) {
	for _, path := range []string{"scrum_play_queue.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(source), "scrumAgentConfigErrorNote") {
			t.Errorf("%s retains content-derived transition narrative", path)
		}
	}
	if _, err := os.Stat("scrum_play_agent.go"); err == nil {
		t.Error("retired Scrum agent adapter remains")
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect retired Scrum agent adapter: %v", err)
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

func TestScrumRuntimeHasNoRetiredStreamReclassificationOrUntypedFallback(t *testing.T) {
	for _, path := range []string{
		"scrum_agent_stream.go",
		"scrum_external_agent_outcome.go",
		"../worker/external_agent.go",
		"../hostbridge/external_agent_client.go",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired stream runtime remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect retired stream runtime %s: %v", path, err)
		}
	}
	for path, forbidden := range map[string][]string{
		"scrum_channel_activity.go": {
			"sdkToolCallToActivity", "agentEventToActivity", "extractFileDiffsFromRaw", "stringFromAnyMap",
			`strings.Contains(eventType, "external_agent")`,
		},
		"scrum_manager.go": {
			"sanitizeScrumChannelText(step.Output)", "sanitizeScrumChannelText(step.Error)",
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
