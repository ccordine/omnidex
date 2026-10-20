package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

type orderingTransport struct {
	calls    int
	response string
}

func (*orderingTransport) Run(context.Context, Assignment, func(NativeEvent) error) (Result, error) {
	return Result{}, fmt.Errorf("unexpected native call")
}
func (p *orderingTransport) GeneratePreparedExact(_ context.Context, prepared llm.PreparedModel) (llm.PreparedGeneration, error) {
	p.calls++
	raw, _ := json.Marshal(map[string]any{"created_at": "2026-10-20T12:00:00Z", "response": p.response, "done": true, "done_reason": "stop", "prompt_eval_count": 40, "eval_count": 12})
	d, err := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, raw)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	return llm.PreparedGeneration{Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol, ProviderRequestDisposition: llm.ProviderRequestDispatched, Content: d.Content, ProviderHTTPStatus: 200, ProviderResponseDisposition: d.Disposition, ProviderResponseComplete: true, ProviderContentEncoding: llm.ProviderEncodingIdentity, ProviderResponseBytesKnown: true, ProviderResponseBytes: int64(len(raw)), ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: raw, ProviderDonePresent: d.DonePresent, ProviderDone: d.Done, ProviderDoneReason: d.DoneReason, UsagePresent: d.UsagePresent, Usage: d.Usage}, nil
}
func TestPlanningOrderingReconstructionAndOwnerEdits(t *testing.T) {
	p := &orderingTransport{response: `["c","a","b"]`}
	journal := &durableTaskEvidence{}
	c := Controller{Generation: 1, Configuration: Configuration{LedgerParser: Route{Provider: "fixture", Model: "fixture"}}, Providers: map[string]Provider{"fixture": p}, Evidence: journal}
	turn := PlanningTurn{OperationID: "owner-op", ObservedRevision: 2}
	tasks := orderingTasks()
	if err := c.orderPlanningTasks(t.Context(), turn, tasks); err != nil {
		t.Fatal(err)
	}
	if tasks[2].Rank != 1 || tasks[0].Rank != 2 || tasks[1].Rank != 3 || p.calls != 1 {
		t.Fatalf("bad order %+v calls%d", tasks, p.calls)
	}
	// New controller, same persisted journal; changed parser slice positions.
	rebuilt := c
	rebuilt.Providers = nil
	replay := []LedgerTask{orderingTasks()[2], orderingTasks()[0], orderingTasks()[1]}
	resumedTurn := PlanningTurn{OperationID: "resumed-owner-op", ObservedRevision: 3}
	if err := rebuilt.orderPlanningTasks(t.Context(), resumedTurn, replay); err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 || replay[0].Rank != 1 || replay[1].Rank != 2 {
		t.Fatal("resolved order lost")
	}
	changed := orderingTasks()
	changed[0].Goal = "A genuinely changed instruction."
	if err := rebuilt.orderPlanningTasks(t.Context(), resumedTurn, changed); err == nil {
		t.Fatal("changed content reused an unrelated resolution")
	}
	current := EngineeringLedger{Tasks: tasks}
	rank := int64(20)
	for _, kind := range []model.LedgerMutationKind{model.LedgerMutationReview, model.LedgerMutationReorder, model.LedgerMutationAmend, model.LedgerMutationRestore, model.LedgerMutationRemove} {
		mutation := LedgerMutation{Operations: []model.LedgerMutationOperation{{Kind: kind, TargetID: "a", Rank: &rank}}}
		before, _ := json.Marshal(mutation)
		if err := rebuilt.orderPlanningAdditions(t.Context(), current, turn, &mutation); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(mutation)
		if string(before) != string(after) {
			t.Fatal("owner operation changed")
		}
	}
	if len(journal.entries) != 1 || journal.entries[0].TaskOrdering.Prompt == "" {
		t.Fatal("missing durable provenance")
	}
}
func TestPlanningOrderingFailureDoesNotResolveOrApply(t *testing.T) {
	p := &orderingTransport{response: `["a","a","b"]`}
	journal := &durableTaskEvidence{}
	c := Controller{Generation: 1, Configuration: Configuration{LedgerParser: Route{Provider: "fixture", Model: "fixture"}}, Providers: map[string]Provider{"fixture": p}, Evidence: journal}
	tasks := orderingTasks()
	before := orderingTasks()
	if err := c.orderPlanningTasks(t.Context(), PlanningTurn{OperationID: "op"}, tasks); err == nil {
		t.Fatal("invalid permutation accepted")
	}
	if !reflect.DeepEqual(tasks, before) || journal.entries[0].Phase != "task_ordering_failed" || journal.entries[0].TaskOrdering.Key != "" {
		t.Fatal("failure became authority")
	}
}

func TestPlanningOrderingOnlyNewAdditionsAndExplicitOrder(t *testing.T) {
	p := &orderingTransport{response: `["e","d"]`}
	journal := &durableTaskEvidence{}
	c := Controller{Generation: 1, Configuration: Configuration{LedgerParser: Route{Provider: "fixture", Model: "fixture"}}, Providers: map[string]Provider{"fixture": p}, Evidence: journal}
	current := EngineeringLedger{Tasks: orderingTasks()}
	turn := PlanningTurn{OperationID: "add", ObservedRevision: 2}
	makeMutation := func() LedgerMutation {
		operations := []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "a", ReviewState: model.LedgerReviewAgreed}}
		for _, id := range []model.LedgerItemID{"d", "e"} {
			task := LedgerTask{LedgerItem: LedgerItem{ID: id, Rank: 1, Disposition: model.LedgerDispositionActive}, Goal: "Exact task " + string(id)}
			operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{Task: &task}})
		}
		return LedgerMutation{Operations: operations}
	}
	mutation := makeMutation()
	if err := c.orderPlanningAdditions(t.Context(), current, turn, &mutation); err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 || mutation.Operations[1].Item.Task.Rank != 32 || mutation.Operations[2].Item.Task.Rank != 31 || mutation.Operations[0].ReviewState != model.LedgerReviewAgreed {
		t.Fatal("existing owner order or new additions lost", mutation)
	}
	explicit := makeMutation()
	rank := int64(1)
	explicit.Operations = append(explicit.Operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReorder, TargetID: "e", Rank: &rank})
	before, _ := json.Marshal(explicit)
	if err := c.orderPlanningAdditions(t.Context(), current, turn, &explicit); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(explicit)
	if string(before) != string(after) || p.calls != 1 {
		t.Fatal("explicit ordering overwritten")
	}
}

func (p *orderingTransport) OrderingClient(context.Context) (llm.ExactStationClient, func(), error) {
	return p, func() {}, nil
}
