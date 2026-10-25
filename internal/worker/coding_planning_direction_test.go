package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestQueuedPlanningRetainsNewerOwnerDirectionWithoutReplacingMessages(t *testing.T) {
	ledger := planningBoundaryLedger()
	message := func(id, text string, generation int64) *queue.ChannelSessionMessage {
		return &queue.ChannelSessionMessage{OperationID: model.LifecycleOperationID(id),
			AdmittedJob: model.Job{CurrentGeneration: generation},
			Command:     queue.ChannelSessionTurnCommand{Text: text}}
	}
	repository := &planningConversationFixture{planningBoundaryRepository: planningBoundaryRepository{
		current: queue.CodingLedgerRevision{LedgerID: 7, Revision: ledger.Revision, Ledger: ledger}},
		pending: []*queue.ChannelSessionMessage{
			message("old", "Retain the temporary fixture through review.", 1),
			message("new", "Keep the pending report task.", 3),
		}}
	provider := &planningConversationProvider{unchanged: true}
	route := codingagent.Route{Provider: "native", Model: "model"}
	controller := &codingagent.Controller{JobID: 1, Generation: 3, Root: t.TempDir(),
		Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route},
		Providers:     map[string]codingagent.Provider{"native": provider}, Environment: planningBoundaryEnvironment{}, Evidence: &planningRetryJournal{}}
	direction := codingagent.PlanningTurn{OperationID: "latest", Text: "Review completed and the fixture was removed. Preserve accepted work; reorder only pending tasks."}
	_, err := finishPlanningConversation(t.Context(), repository, model.StepAttemptAuthority{Generation: 3}, controller, repository.current, direction)
	if err != nil || len(repository.resolved) != 2 || len(provider.calls) != 4 {
		t.Fatalf("queued conversation did not finish: %v, calls=%d, resolved=%v", err, len(provider.calls), repository.resolved)
	}
	old, next := provider.calls[0], provider.calls[2]
	if old.Objective.Instruction != "Retain the temporary fixture through review." || old.NewerOwnerDirection == nil ||
		old.NewerOwnerDirection.OperationID != direction.OperationID || old.NewerOwnerDirection.Text != direction.Text {
		t.Fatalf("older message lost its source or later authority: %+v", old)
	}
	if next.Objective.Instruction != "Keep the pending report task." || next.NewerOwnerDirection != nil {
		t.Fatalf("later reply was incorrectly superseded: %+v", next)
	}
	if !strings.Contains(provider.calls[1].ConstructionFacts[1].Quote, direction.Text) {
		t.Fatal("planning parser did not receive the same newer authority")
	}
}
