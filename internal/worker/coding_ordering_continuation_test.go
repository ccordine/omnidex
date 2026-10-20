package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type unchangedOrderingRepository struct {
	current  queue.CodingLedgerRevision
	proposed bool
}

func (r *unchangedOrderingRepository) ReadCodingLedger(context.Context, int64, queue.CodingLedgerReadOptions) (queue.CodingLedgerRevision, error) {
	return r.current, nil
}
func (*unchangedOrderingRepository) CreateCodingLedger(context.Context, queue.CreateCodingLedgerCommand) (queue.CodingLedgerRevision, error) {
	return queue.CodingLedgerRevision{}, fmt.Errorf("unexpected create")
}
func (*unchangedOrderingRepository) ApplyCodingLedgerMutation(context.Context, queue.ApplyCodingLedgerMutationCommand) (queue.CodingLedgerRevision, error) {
	return queue.CodingLedgerRevision{}, fmt.Errorf("unexpected mutation")
}
func (r *unchangedOrderingRepository) ProposeUnchangedCodingLedger(_ context.Context, command queue.ProposeUnchangedCodingLedgerCommand) (queue.CodingLedgerRevision, error) {
	if command.Revision != r.current.Revision {
		return queue.CodingLedgerRevision{}, fmt.Errorf("stale")
	}
	r.proposed = true
	r.current.State = "proposed"
	r.current.Revision++
	r.current.Ledger.Revision++
	return r.current, nil
}

type unchangedOrderingJournal struct{ entries []codingagent.Entry }

func (j *unchangedOrderingJournal) Read(context.Context) ([]codingagent.Entry, error) {
	return j.entries, nil
}
func (j *unchangedOrderingJournal) Append(_ context.Context, e codingagent.Entry) (int64, error) {
	j.entries = append(j.entries, e)
	return int64(len(j.entries)), nil
}
func (*unchangedOrderingJournal) ReadTaskClarifications(context.Context, string) ([]codingagent.TaskClarification, error) {
	return nil, nil
}

type unchangedOrderingProvider struct{}

func (unchangedOrderingProvider) Attest(context.Context) error { return nil }
func (unchangedOrderingProvider) Run(_ context.Context, c codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"unchanged"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	content := json.RawMessage(`"Keep the unchanged proposal."`)
	if c.Role == codingagent.PlanningMutation {
		content = json.RawMessage(`{"operations":[]}`)
	}
	return codingagent.Result{Provider: c.Route.Provider, Model: c.Route.Model, Content: content, Completed: true, ThreadID: "fixture", TurnID: "fixture"}, nil
}
func TestOrderingUnchangedContinuationProposesRevisingLedger(t *testing.T) {
	task := model.LedgerTask{LedgerItem: model.LedgerItem{ID: "retained", Rank: 7, Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewAgreed, Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "retained", Revision: 1}}, Goal: "Retain accepted instructions.", ReadFiles: []string{}, WriteFiles: []string{}, DeleteFiles: []string{}}
	r := &unchangedOrderingRepository{current: queue.CodingLedgerRevision{LedgerID: 1, Revision: 4, State: "revising", Ledger: model.EngineeringLedger{ID: "1", Revision: 4, Tasks: []model.LedgerTask{task}}}}
	route := codingagent.Route{Provider: "fixture", Model: "fixture"}
	p := unchangedOrderingProvider{}
	c := &codingagent.Controller{JobID: 1, Generation: 2, Root: "/fixture", Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}, Environment: p, Providers: map[string]codingagent.Provider{"fixture": p}, Evidence: &unchangedOrderingJournal{}}
	turn := codingagent.PlanningTurn{OperationID: "continue", OwnerTurnID: "owner-continue", Text: "Keep unchanged.", ObservedRevision: 4}
	result, err := applyCodingLedgerPlanningTransaction(t.Context(), r, c, 1, turn, nil)
	if err != nil || !r.proposed || result.State != "proposed" || result.Ledger.Tasks[0].Rank != 7 {
		t.Fatalf("unchanged continuation stranded in revising: %+v %v", result, err)
	}
	// This provider deliberately has no ordering transport. No inference is needed.
}
