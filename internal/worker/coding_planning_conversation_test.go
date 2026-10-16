package worker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type planningConversationFixture struct {
	planningBoundaryRepository
	pending  []*queue.ChannelSessionMessage
	resolved []model.LifecycleOperationID
	racing   *queue.ChannelSessionMessage
	waits    int
}

func (repository *planningConversationFixture) NextChannelSessionMessage(ctx context.Context, _ model.StepAttemptAuthority) (*queue.ChannelSessionMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(repository.pending) == 0 {
		return nil, nil
	}
	return repository.pending[0], nil
}
func (repository *planningConversationFixture) ResolvePlanningSessionMessage(_ context.Context, _ model.StepAttemptAuthority, id model.LifecycleOperationID) (queue.ChannelSessionMessageResult, error) {
	repository.resolved = append(repository.resolved, id)
	repository.pending = repository.pending[1:]
	return queue.ChannelSessionMessageResult{Applied: true}, nil
}
func (*planningConversationFixture) CodingLedgerRevisionForOperation(context.Context, int64, string) (int64, bool, error) {
	return 0, false, nil
}
func (repository *planningConversationFixture) MarkCodingLedgerReviewReady(_ context.Context, _ model.StepAttemptAuthority, _ int64, revision int64) error {
	repository.waits++
	if repository.racing != nil {
		repository.pending = append(repository.pending, repository.racing)
		repository.racing = nil
		return queue.ErrPendingChannelSessionMessage
	}
	if revision != repository.current.Revision {
		return errors.New("wait used stale ledger")
	}
	return nil
}

type planningConversationProvider struct {
	calls           []codingagent.Assignment
	interrupt       bool
	invalidMutation bool
}

func (provider *planningConversationProvider) Run(_ context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	provider.calls = append(provider.calls, call)
	if provider.interrupt {
		return codingagent.Result{}, errors.New("interrupted")
	}
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"observed":true}`)}); err != nil {
		return codingagent.Result{}, err
	}
	content := json.RawMessage(`"I updated the task order as requested."`)
	if call.Role == codingagent.PlanningMutation {
		content = json.RawMessage(`{"operations":[{"kind":"reorder","target_id":"task","rank":10,"rationale":""}]}`)
		if provider.invalidMutation {
			content = json.RawMessage(`{"operations":[{"kind":"remove","target_id":"","rationale":""}]}`)
		}
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: "thread", TurnID: "turn", Content: content}, nil
}

type planningRetryJournal struct{ entries []codingagent.Entry }

func (journal *planningRetryJournal) Append(_ context.Context, entry codingagent.Entry) (int64, error) {
	entry.ID = int64(len(journal.entries) + 1)
	journal.entries = append(journal.entries, entry)
	return entry.ID, nil
}
func (journal *planningRetryJournal) Read(context.Context) ([]codingagent.Entry, error) {
	return journal.entries, nil
}
func (*planningRetryJournal) ReadTaskClarifications(context.Context, string) ([]codingagent.TaskClarification, error) {
	return nil, nil
}

func TestPlanningConversationRetainsQueuedReplyAfterSemanticParserFailure(t *testing.T) {
	ledger := planningBoundaryLedger()
	repository := &planningConversationFixture{planningBoundaryRepository: planningBoundaryRepository{current: queue.CodingLedgerRevision{LedgerID: 7, Revision: ledger.Revision, Ledger: ledger}},
		pending: []*queue.ChannelSessionMessage{{OperationID: "unfinished", Command: queue.ChannelSessionTurnCommand{Text: "Move the task."}}}}
	provider := &planningConversationProvider{invalidMutation: true}
	route := codingagent.Route{Provider: "native", Model: "model"}
	controller := &codingagent.Controller{JobID: 1, Generation: 1, Root: t.TempDir(),
		Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route},
		Requirements:  []codingagent.Requirement{{ID: "objective", Outcome: "ship"}}, Providers: map[string]codingagent.Provider{"native": provider}, Environment: planningBoundaryEnvironment{}, Evidence: &planningRetryJournal{}}
	_, err := finishPlanningConversation(context.Background(), repository, model.StepAttemptAuthority{}, controller, repository.current)
	var failure *codingagent.ParserFailure
	if !errors.As(err, &failure) || len(repository.pending) != 1 || len(repository.resolved) != 0 || repository.current.Revision != ledger.Revision {
		t.Fatalf("bad parse lost owner reply or mutated authority: %+v %v", repository, err)
	}
	provider.invalidMutation = false
	result, err := finishPlanningConversation(context.Background(), repository, model.StepAttemptAuthority{}, controller, repository.current)
	if err != nil || len(repository.pending) != 0 || len(repository.resolved) != 1 || result.Revision != ledger.Revision+1 {
		t.Fatalf("parser recovery did not finish the same queued turn: %+v %v", repository, err)
	}
	plans, parses := 0, 0
	for _, call := range provider.calls {
		if call.Role == codingagent.Plan {
			plans++
		}
		if call.Role == codingagent.PlanningMutation {
			parses++
		}
	}
	if plans != 1 || parses != 2 {
		t.Fatalf("parser retry repeated native planning: %d/%d", plans, parses)
	}
}

func TestPlanningConversationDrainsFIFOAndRacingReplyWithoutClassifierOrReplan(t *testing.T) {
	message := func(id string) *queue.ChannelSessionMessage {
		return &queue.ChannelSessionMessage{OperationID: model.LifecycleOperationID(id), Command: queue.ChannelSessionTurnCommand{Text: id}}
	}
	ledger := planningBoundaryLedger()
	repository := &planningConversationFixture{planningBoundaryRepository: planningBoundaryRepository{current: queue.CodingLedgerRevision{LedgerID: 7, Revision: ledger.Revision, Ledger: ledger}}, pending: []*queue.ChannelSessionMessage{message("first"), message("second")}, racing: message("third")}
	provider := &planningConversationProvider{}
	route := codingagent.Route{Provider: "native", Model: "model"}
	controller := &codingagent.Controller{JobID: 1, Generation: 1, Root: t.TempDir(),
		Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route},
		Requirements:  []codingagent.Requirement{{ID: "objective", Outcome: "ship"}}, Providers: map[string]codingagent.Provider{"native": provider}, Environment: planningBoundaryEnvironment{}, Evidence: planningBoundaryEvidence{}}
	result, err := finishPlanningConversation(context.Background(), repository, model.StepAttemptAuthority{}, controller, repository.current)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(repository.resolved, []model.LifecycleOperationID{"first", "second", "third"}) || result.Revision != ledger.Revision+3 || repository.waits != 2 || len(provider.calls) != 6 {
		t.Fatalf("conversation lost/redirected work: %+v calls=%d", repository, len(provider.calls))
	}
	for index := 0; index < 3; index++ {
		plan, parser := provider.calls[2*index], provider.calls[2*index+1]
		if plan.Role != codingagent.Plan || plan.Objective.Instruction != string(repository.resolved[index]) || plan.PlanningLedger.Revision != ledger.Revision+int64(index) || parser.Role != codingagent.PlanningMutation || parser.PlanningBaseRevision != ledger.Revision+int64(index) {
			t.Fatalf("wrong planning context/order: %+v %+v", plan, parser)
		}
	}
	provider.interrupt = true
	repository.pending = []*queue.ChannelSessionMessage{message("unfinished")}
	if _, err := finishPlanningConversation(context.Background(), repository, model.StepAttemptAuthority{}, controller, repository.current); err == nil || len(repository.pending) != 1 || len(repository.resolved) != 3 {
		t.Fatalf("interruption consumed unfinished reply: %+v %v", repository, err)
	}
}

func TestNativeAgentsPlanningDoesNotInstallGenericMessageClassifier(t *testing.T) {
	route := codingagent.Route{Provider: "native", Model: "model"}
	metadata, err := json.Marshal(map[string]any{"coding_execution": codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &nativeRuntimeV3{svc: &Service{}, action: "v3_coding_plan", claim: &model.ClaimedStep{Job: model.Job{Pipeline: model.PipelineChat, Metadata: metadata}}}
	consumer, err := newChannelSessionMessageConsumer(runtime)
	if err != nil || consumer != nil {
		t.Fatalf("native planning installed generic consumer: %v %v", consumer, err)
	}
}
