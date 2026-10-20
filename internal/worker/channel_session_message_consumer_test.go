package worker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestActiveSessionConsumerAnswersWithoutChangingClaim(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	original := *claim
	classifications, responses := 0, 0
	consumer.kind = sessionKindFunc(func(input assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
		classifications++
		if len(input.Context.Capsules) != 0 || input.Context.ReplanAuthority != nil ||
			input.ExactInstruction != "Explain ARTIFACT_1 and ARTIFACT_2 without changing the task." {
			t.Fatalf("classifier received parent task or wrong text: %+v", input)
		}
		return assemblyline.ConversationObjectiveKindDecision{Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindAnswer}, 0, nil
	})
	consumer.response = usageConversationFunc(func(input assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		responses++
		prompt, err := assemblyline.BuildConversationResponsePrompt(input)
		if err != nil {
			t.Fatal(err)
		}
		if err := assemblyline.ValidatePathFreeModelContext("active session answer", prompt); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(prompt, claim.Job.Instruction) || !strings.Contains(prompt, "Current task status: running") ||
			!strings.Contains(prompt, "Current step action: v3_coding") {
			t.Fatalf("answer lacks bounded current task facts: %s", prompt)
		}
		decision, err := assemblyline.DecodeConversationResponseDecision(input, "ARTIFACT_1 pauses; ARTIFACT_2 stops.")
		return decision, 0, err
	})
	consumed, err := consumer.consume(context.Background())
	if err != nil || !consumed || classifications != 1 || responses != 1 {
		t.Fatalf("pending question was not answered: consumed=%t classify=%d response=%d err=%v", consumed, classifications, responses, err)
	}
	if !reflect.DeepEqual(original, *claim) || len(store.resolutions) != 1 ||
		store.resolutions[0].Kind != assemblyline.ObjectiveKindAnswer ||
		store.resolutions[0].Response != "/interrupt pauses; /stop stops." {
		t.Fatalf("answer changed task authority or lost exact references: claim=%+v resolutions=%+v", claim, store.resolutions)
	}
}

func TestActiveSessionConsumerReplansOnlyWorkspaceMutation(t *testing.T) {
	consumer, store, _ := activeSessionConsumerTest(t)
	consumer.kind = sessionKindFunc(func(assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
		return assemblyline.ConversationObjectiveKindDecision{Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindWorkspaceMutation}, 0, nil
	})
	consumer.response = usageConversationFunc(func(assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		t.Fatal("workspace mutation invoked a conversational response")
		return assemblyline.ConversationResponseDecision{}, 0, nil
	})
	store.replanned = true
	consumed, err := consumer.consume(context.Background())
	if !consumed || !errors.Is(err, queue.ErrStaleStepAttempt) || len(store.resolutions) != 1 ||
		store.resolutions[0].Kind != assemblyline.ObjectiveKindWorkspaceMutation || store.resolutions[0].Response != "" {
		t.Fatalf("mutation did not retire only the old runtime: consumed=%t resolutions=%+v err=%v", consumed, store.resolutions, err)
	}
}

func TestActiveSessionConsumerRejectsCancelledAndLostAuthority(t *testing.T) {
	consumer, store, _ := activeSessionConsumerTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := consumer.consume(ctx); !errors.Is(err, context.Canceled) || store.reads != 0 {
		t.Fatalf("cancelled runtime opened pending work: reads=%d err=%v", store.reads, err)
	}
	store.authorityErr = queue.ErrStaleStepAttempt
	if _, err := consumer.consume(context.Background()); !errors.Is(err, queue.ErrStaleStepAttempt) || store.reads != 0 {
		t.Fatalf("lost authority opened pending work: reads=%d err=%v", store.reads, err)
	}
}

func TestActiveSessionConsumerSerializesOneMessagePerBoundary(t *testing.T) {
	consumer, store, _ := activeSessionConsumerTest(t)
	consumer.kind = sessionKindFunc(func(assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
		return assemblyline.ConversationObjectiveKindDecision{Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindAnswer}, 0, nil
	})
	consumer.response = usageConversationFunc(func(input assemblyline.ConversationResponseInput) (assemblyline.ConversationResponseDecision, int, error) {
		decision, err := assemblyline.DecodeConversationResponseDecision(input, "An answer.")
		return decision, 0, err
	})
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := consumer.consume(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if len(store.resolutions) != 1 || store.reads != 2 {
		t.Fatalf("parallel boundaries duplicated a pending turn: reads=%d resolutions=%+v", store.reads, store.resolutions)
	}
}

type sessionKindFunc func(assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error)

func (classify sessionKindFunc) Classify(_ context.Context, input assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
	return classify(input)
}

type sessionMessageStoreProbe struct {
	pending      *queue.ChannelSessionMessage
	reads        int
	authorityErr error
	resolutions  []queue.ResolveChannelSessionMessageCommand
	replanned    bool
	afterResolve func()
	afterRead    func()
	failures     []string
	failureErr   error
}

func (store *sessionMessageStoreProbe) RequireActiveStepAttempt(context.Context, model.StepAttemptAuthority) error {
	return store.authorityErr
}

func (store *sessionMessageStoreProbe) NextChannelSessionMessage(context.Context, model.StepAttemptAuthority) (*queue.ChannelSessionMessage, error) {
	store.reads++
	message := store.pending
	if store.afterRead != nil {
		store.afterRead()
	}
	return message, nil
}

func (store *sessionMessageStoreProbe) ResolveChannelSessionMessage(_ context.Context, command queue.ResolveChannelSessionMessageCommand) (queue.ChannelSessionMessageResult, error) {
	store.resolutions = append(store.resolutions, command)
	store.pending = nil
	if store.afterResolve != nil {
		store.afterResolve()
	}
	return queue.ChannelSessionMessageResult{Replanned: store.replanned}, nil
}

func (store *sessionMessageStoreProbe) FailChannelSessionMessage(_ context.Context, command queue.FailChannelSessionMessageCommand) (queue.ChannelSessionMessageResult, error) {
	if store.failureErr != nil {
		return queue.ChannelSessionMessageResult{}, store.failureErr
	}
	store.failures = append(store.failures, command.Error)
	store.pending = nil
	if store.afterResolve != nil {
		store.afterResolve()
	}
	return queue.ChannelSessionMessageResult{}, nil
}

func activeSessionConsumerTest(t *testing.T) (*channelSessionMessageConsumer, *sessionMessageStoreProbe, *model.ClaimedStep) {
	t.Helper()
	job := model.Job{ID: 17, CurrentGeneration: 1, Pipeline: model.PipelineChat,
		Status: model.JobStatusRunning, Instruction: "Build the selected application.",
		Metadata: []byte(`{"channel_id":"chat-test","channel_mode":"assistant","client_workspace_identity":"` + commandContextWorkspaceIdentity + `"}`)}
	claim := &model.ClaimedStep{Job: job,
		Step:      model.Step{ID: 41, JobID: 17, Generation: 1, Action: "v3_coding", Status: model.StepStatusRunning},
		Authority: model.StepAttemptAuthority{JobID: 17, Generation: 1, StepID: 41, Attempt: 1, WorkerID: "test-worker"}}
	operation, err := model.ParseLifecycleOperationID("lifecycle_operation_active_message_test")
	if err != nil {
		t.Fatal(err)
	}
	store := &sessionMessageStoreProbe{pending: &queue.ChannelSessionMessage{
		Command: queue.ChannelSessionTurnCommand{OperationID: operation,
			Text: "Explain /interrupt and /stop without changing the task."}, AdmittedJob: job}}
	return &channelSessionMessageConsumer{claim: claim, repository: store}, store, claim
}

func TestActiveSessionConsumerQueuesAmendmentWithoutCancellingTask(t *testing.T) {
	consumer, store, claim := activeSessionConsumerTest(t)
	original := *claim
	store.pending.PlanningBinding = &queue.CodingPlanningBinding{LedgerID: 28, Revision: 60}
	consumer.kind = sessionKindFunc(func(assemblyline.ConversationObjectiveKindInput) (assemblyline.ConversationObjectiveKindDecision, int, error) {
		return assemblyline.ConversationObjectiveKindDecision{Schema: assemblyline.ConversationObjectiveKindSchemaV1, Kind: assemblyline.ObjectiveKindWorkspaceMutation}, 0, nil
	})
	consumed, err := consumer.consume(t.Context())
	if err != nil || !consumed || !reflect.DeepEqual(original, *claim) || len(store.resolutions) != 1 {
		t.Fatalf("amendment interrupted accepted execution: %+v %v", claim, err)
	}
}
