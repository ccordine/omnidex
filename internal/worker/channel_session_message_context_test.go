package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestActiveSessionTaskContextUsesRequiredMinificationOnlyWhenNeeded(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	claim.Job.Instruction = strings.Repeat("Preserve every accepted part of the application. ", 60)
	store.pending.AdmittedJob = claim.Job
	minifications := 0
	consumer.minification = sessionMinificationFunc(func(input assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error) {
		minifications++
		if input.ExactInstruction != "Explain ARTIFACT_1 and ARTIFACT_2 without changing the task." || len(input.SelectedAuthorities) < 2 {
			t.Fatalf("task minification changed instruction or lost required chunks: %+v", input)
		}
		for _, candidate := range input.SelectedAuthorities {
			if candidate.Namespace != "session_current_task" {
				t.Fatalf("task context expanded to unrelated sources: %+v", candidate)
			}
		}
		decision, err := assemblyline.DecodeContextMinificationDecision(input, "The running task preserves accepted application work.")
		return decision, 0, err
	})
	consumer.response = usageConversationFunc(func(input assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		if len(input.Context.Capsules) != 1 || input.Context.Capsules[0].Content != "The running task preserves accepted application work." {
			t.Fatalf("answer did not consume bounded required task context: %+v", input.Context)
		}
		decision, err := assemblyline.DecodeConversationResponseDecision(input, "An answer.")
		return decision, 0, err
	})
	if _, err := consumer.consume(context.Background()); err != nil || minifications != 1 {
		t.Fatalf("required task reduction: calls=%d err=%v", minifications, err)
	}
}

func TestActiveSessionTaskContextFailsTruthfullyWithoutRequiredMinification(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	claim.Job.Instruction = strings.Repeat("Preserve every accepted part of the application. ", 60)
	store.pending.AdmittedJob = claim.Job
	if consumed, err := consumer.consume(context.Background()); err != nil || !consumed || store.pending != nil ||
		len(store.resolutions) != 0 || len(store.failures) != 1 || !strings.Contains(store.failures[0], "minification station is unavailable") {
		t.Fatalf("missing required projection was silently replaced: pending=%t resolutions=%d failures=%v err=%v", store.pending != nil, len(store.resolutions), store.failures, err)
	}
}

func TestActiveSessionTaskContextUsesCurrentRunningStatusAfterPendingAdmission(t *testing.T) {
	consumer, store, _ := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	store.pending.AdmittedJob.Status = "pending"
	consumer.response = usageConversationFunc(func(input assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		content := input.Context.Capsules[0].Content
		if !strings.Contains(content, "Current task status: running") || strings.Contains(content, "Current task status: pending") {
			t.Fatalf("current task facts reused stale intake status: %s", content)
		}
		decision, err := assemblyline.DecodeConversationResponseDecision(input, "An answer.")
		return decision, 0, err
	})
	if _, err := consumer.consume(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeWaitDoesNotStartNewWorkAfterParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := &nativeRuntimeV3{}
	if err := runtime.runWithSessionMessages(ctx, func(context.Context) error {
		t.Fatal("cancelled parent started native work")
		return nil
	}); err == nil {
		t.Fatal("cancelled parent lost its failure")
	}
}

type sessionMinificationFunc func(assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error)

func (minify sessionMinificationFunc) Minify(_ context.Context, input assemblyline.ContextMinificationInput) (assemblyline.ContextMinificationDecision, int, error) {
	return minify(input)
}
