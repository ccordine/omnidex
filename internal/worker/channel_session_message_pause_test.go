package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/queue"
)

func TestNativeWaitDrainsLateMessageBeforeReturningQuestions(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	late := store.pending
	store.pending = nil
	firstRead, admitted := make(chan struct{}), make(chan struct{})
	store.afterRead = func() {
		if store.reads == 1 {
			close(firstRead)
			<-admitted
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, sessionMessages: consumer}
	questions := &codingagent.QuestionsPending{Questions: []string{"Which approved option should be used?"}}
	assignments := 0
	err := runtime.runWithSessionMessages(ctx, func(context.Context) error {
		assignments++
		<-firstRead
		store.pending = late
		close(admitted)
		return questions
	})
	if err != questions || assignments != 1 || store.pending != nil || len(store.resolutions) != 1 {
		t.Fatalf("native question pause orphaned late message or repeated assignment: assignments=%d pending=%t responses=%d err=%v", assignments, store.pending != nil, len(store.resolutions), err)
	}
}

func TestNativeWaitLateMutationRetiresOriginalPause(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	consumer.kind = sessionKindFunc(func(assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
		return assemblyline.ConversationObjectiveKindDecision{Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindWorkspaceMutation}, 0, nil
	})
	store.replanned = true
	late := store.pending
	store.pending = nil
	firstRead, admitted := make(chan struct{}), make(chan struct{})
	store.afterRead = func() {
		if store.reads == 1 {
			close(firstRead)
			<-admitted
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, sessionMessages: consumer}
	err := runtime.runWithSessionMessages(ctx, func(context.Context) error {
		<-firstRead
		store.pending = late
		close(admitted)
		return &codingagent.QuestionsPending{Questions: []string{"Which approved option should be used?"}}
	})
	if !errors.Is(err, queue.ErrStaleStepAttempt) || store.pending != nil || len(store.resolutions) != 1 {
		t.Fatalf("late redirect returned an obsolete question pause: pending=%t resolutions=%d err=%v", store.pending != nil, len(store.resolutions), err)
	}
}

func TestPauseTransitionDrainsMessageRacingWithCommit(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	late := *store.pending
	late.Command.OperationID = "lifecycle_operation_pause_boundary_late"
	store.pending = nil
	runtime := &nativeRuntimeV3{ctx: context.Background(), claim: claim, sessionMessages: consumer}
	questions := []string{"Which approved option should be used?"}
	commits := 0
	err := runtime.transitionAfterSessionMessages(runtime.ctx, func() error {
		commits++
		if questions[0] != "Which approved option should be used?" {
			t.Fatal("message drain changed the original pause command")
		}
		if commits == 1 {
			store.pending = &late
			return queue.ErrPendingChannelSessionMessage
		}
		return nil
	})
	if err != nil || commits != 2 || len(store.resolutions) != 1 {
		t.Fatalf("pause race lost message or recreated work: commits=%d responses=%d err=%v", commits, len(store.resolutions), err)
	}
}
