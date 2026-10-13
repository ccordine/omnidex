package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestAcceptedPortableBoundaryConsumesOnlyAfterAcceptance(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, sessionMessages: consumer}
	failure := errors.New("exact station persistence failed")
	finalize := runtime.afterAcceptedPortableResult(runtime.ctx, func(assemblyline.PortableJob, assemblyline.PortableResult, error) error {
		return failure
	})
	if err := finalize(assemblyline.PortableJob{}, assemblyline.PortableResult{}, nil); !errors.Is(err, failure) || store.reads != 0 {
		t.Fatalf("failed leaf opened conversation: reads=%d err=%v", store.reads, err)
	}
	finalize = runtime.afterAcceptedPortableResult(runtime.ctx, func(assemblyline.PortableJob, assemblyline.PortableResult, error) error { return nil })
	if err := finalize(assemblyline.PortableJob{}, assemblyline.PortableResult{}, failure); err != nil || store.reads != 0 {
		t.Fatalf("rejected leaf opened conversation: reads=%d err=%v", store.reads, err)
	}
	if err := finalize(assemblyline.PortableJob{}, assemblyline.PortableResult{}, nil); err != nil || len(store.resolutions) != 1 {
		t.Fatalf("accepted leaf did not answer pending turn: resolutions=%+v err=%v", store.resolutions, err)
	}
}

func TestStepCompletionDrainsMessageRacingWithPreflight(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	next := *store.pending
	next.Command.OperationID = "lifecycle_operation_active_message_second"
	runtime := &nativeRuntimeV3{ctx: context.Background(), claim: claim, sessionMessages: consumer}
	commits := 0
	err := runtime.transitionAfterSessionMessages(runtime.ctx, func() error {
		commits++
		if commits == 1 {
			store.pending = &next
			return queue.ErrPendingChannelSessionMessage
		}
		return nil
	})
	if err != nil || commits != 2 || len(store.resolutions) != 2 {
		t.Fatalf("completion left queued question behind: commits=%d responses=%d err=%v", commits, len(store.resolutions), err)
	}
}

func TestNativeWaitConsumesPendingQuestionBeforeReturning(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	started, answered := make(chan struct{}), make(chan struct{})
	store.afterResolve = func() { <-started; close(answered) }
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, sessionMessages: consumer}
	err := runtime.runWithSessionMessages(runtime.ctx, func(ctx context.Context) error {
		close(started)
		select {
		case <-answered:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil || len(store.resolutions) != 1 {
		t.Fatalf("native wait held the ordinary question: responses=%d err=%v", len(store.resolutions), err)
	}
}

func TestNativeWaitJoinsItsCancelledTaskAfterMessageFailurePersistenceError(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	setSessionAnswerStations(consumer)
	failure := errors.New("observed conversation provider failure")
	store.failureErr = errors.New("message failure could not be persisted")
	consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		return assemblyline.ConversationResponseDecision{}, 1, failure
	})
	stopped := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runtime := &nativeRuntimeV3{ctx: ctx, claim: claim, sessionMessages: consumer}
	err := runtime.runWithSessionMessages(runtime.ctx, func(ctx context.Context) error {
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	if !errors.Is(err, failure) || store.pending == nil || len(store.resolutions) != 0 {
		t.Fatalf("genuine persistence failure became a successful or cancelled answer: pending=%t err=%v", store.pending != nil, err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("native task survived the failed conversation boundary")
	}
}

func TestActiveSessionCancellationDuringClassificationPreventsResponseAndCommit(t *testing.T) {
	consumer, store, _ := activeSessionConsumerTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumer.kind = sessionKindFunc(func(assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
		cancel()
		return assemblyline.ConversationObjectiveKindDecision{Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindAnswer}, 0, nil
	})
	consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		t.Fatal("cancelled classification opened a new response call")
		return assemblyline.ConversationResponseDecision{}, 0, nil
	})
	if _, err := consumer.consume(ctx); !errors.Is(err, context.Canceled) || len(store.resolutions) != 0 {
		t.Fatalf("cancelled classification committed a turn: resolutions=%+v err=%v", store.resolutions, err)
	}
}

func TestActiveSessionContextBindsTaskArtifactsWithoutGrantingUnreferencedCommands(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	claim.Job.Instruction = "Preserve /private/saves and explain /exit after the task."
	store.pending.AdmittedJob = claim.Job
	setSessionAnswerStations(consumer)
	consumer.response = usageConversationFunc(func(input assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		if len(input.CLICommands) != 2 {
			t.Fatalf("task request supplied an unreferenced CLI meaning: %+v", input.CLICommands)
		}
		prompt, err := assemblyline.BuildConversationResponsePrompt(input)
		if err != nil {
			t.Fatal(err)
		}
		if err := assemblyline.ValidatePathFreeModelContext("current task projection", prompt); err != nil {
			t.Fatal(err)
		}
		decision, err := assemblyline.DecodeConversationResponseDecision(input, "The task preserves ARTIFACT_3.")
		return decision, 0, err
	})
	if _, err := consumer.consume(context.Background()); err != nil ||
		len(store.resolutions) != 1 || store.resolutions[0].Response != "The task preserves /private/saves." {
		t.Fatalf("current task facts lost literal provenance: resolutions=%+v err=%v", store.resolutions, err)
	}
}

func setSessionAnswerStations(consumer *channelSessionMessageConsumer) {
	consumer.kind = sessionKindFunc(func(assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
		return assemblyline.ConversationObjectiveKindDecision{Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindAnswer}, 0, nil
	})
	consumer.response = usageConversationFunc(func(input assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		decision, err := assemblyline.DecodeConversationResponseDecision(input, "An answer.")
		return decision, 0, err
	})
}
