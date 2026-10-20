package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

// Scripted planning and native work exercise real ledger/controller mechanics;
// they make no natural-language ordering claim.
type proposalOrderingProvider struct {
	ledger      model.EngineeringLedger
	ordering    int
	operations  []model.LedgerMutationOperation
	duringOrder func()
}

func (*proposalOrderingProvider) Attest(context.Context) error { return nil }
func (p *proposalOrderingProvider) Run(_ context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"proposal"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	content := json.RawMessage(`"Draft intake has z before a."`)
	if call.Role == codingagent.PlanningInitial {
		content, _ = json.Marshal(map[string]any{"ledger": p.ledger})
	}
	if call.Role == codingagent.PlanningMutation {
		content, _ = json.Marshal(map[string]any{"operations": p.operations})
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true, ThreadID: "fixture-thread", TurnID: "fixture-turn", Content: content}, nil
}
func (p *proposalOrderingProvider) GeneratePreparedExact(_ context.Context, prepared llm.PreparedModel) (llm.PreparedGeneration, error) {
	p.ordering++
	if p.duringOrder != nil {
		p.duringOrder()
	}
	response := `["a","z"]`
	if strings.Contains(prepared.Prompt, `"id":"b"`) {
		response = `["b","c"]`
	}
	raw, _ := json.Marshal(map[string]any{"created_at": "2026-10-20T12:00:00Z", "response": response, "done": true, "done_reason": "stop", "prompt_eval_count": 40, "eval_count": 12})
	d, err := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, raw)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	return llm.PreparedGeneration{Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol, ProviderRequestDisposition: llm.ProviderRequestDispatched, Content: d.Content, ProviderHTTPStatus: 200, ProviderResponseDisposition: d.Disposition, ProviderResponseComplete: true, ProviderContentEncoding: llm.ProviderEncodingIdentity, ProviderResponseBytesKnown: true, ProviderResponseBytes: int64(len(raw)), ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: raw, ProviderDonePresent: d.DonePresent, ProviderDone: d.Done, ProviderDoneReason: d.DoneReason, UsagePresent: d.UsagePresent, Usage: d.Usage}, nil
}
func proposalTask(id string, rank int64) model.LedgerTask {
	return model.LedgerTask{LedgerItem: model.LedgerItem{ID: model.LedgerItemID(id), Rank: rank, Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed, Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "fixture", Revision: 1}}, Goal: "Perform exact task " + id, ReadFiles: []string{}, WriteFiles: []string{"fixture.txt"}, DeleteFiles: []string{}}
}

func TestProposalOrderingPersistPublishReplayAndPostgreSQLSelection(t *testing.T) {
	for _, ties := range []bool{false, true} {
		t.Run(fmt.Sprint("ties=", ties), func(t *testing.T) {
			pool, repo := freshUserIntentRepository(t)
			ctx := t.Context()
			root := t.TempDir()
			identity, err := projectroot.DirectoryIdentity(root)
			if err != nil {
				t.Fatal(err)
			}
			identity, err = projectroot.ClientWorkspaceIdentity("11111111111111111111111111111111", identity)
			if err != nil {
				t.Fatal(err)
			}
			channel, err := repo.EnsureCLIChatSessionChannel(ctx, root, identity)
			if err != nil {
				t.Fatal(err)
			}
			command := ChannelSessionTurnCommand{OperationID: recoveryOperationID(t), ChannelID: channel.ID, WorkspaceRoot: root, WorkspaceIdentity: identity, CodingMode: codingagent.Agents, Text: "Do these two exact tasks."}
			initial, err := repo.SubmitChannelSessionTurn(ctx, command)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repo.ClaimNextStep(ctx, "proposal-planner")
			if err != nil || claim == nil {
				t.Fatal(err)
			}
			p := &proposalOrderingProvider{ledger: model.EngineeringLedger{ID: "proposal", Revision: 1, Tasks: []model.LedgerTask{proposalTask("z", 10), proposalTask("a", 20)}}}
			c := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Job.CurrentGeneration, Root: root, Configuration: repo.codingAgents, Environment: p, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": p}}
			turn := codingagent.PlanningTurn{OperationID: "initial", OwnerTurnID: "owner-initial", Text: command.Text}
			proposal, err := c.ProposePlanningLedger(ctx, turn)
			if err != nil {
				t.Fatal(err)
			}
			if p.ordering != 1 || proposal.Ledger.Tasks[0].Rank != 2 || proposal.Ledger.Tasks[1].Rank != 1 {
				t.Fatal("ordering not applied", proposal)
			}
			var messages int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_channel_messages WHERE channel_id=$1 AND role='assistant'`, channel.ID).Scan(&messages); err != nil || messages != 0 {
				t.Fatal("draft published before persistence", messages, err)
			}
			// Reconstruct before commit: the persisted semantic receipt must suppress inference.
			c.Providers = nil
			replay, err := c.ProposePlanningLedger(ctx, turn)
			// Parsing still needs its configured provider; ordering itself must reuse evidence.
			if err == nil {
				t.Fatal("missing parser provider unexpectedly accepted")
			}
			c.Providers = map[string]codingagent.Provider{"codex": p}
			replay, err = c.ProposePlanningLedger(ctx, turn)
			if err != nil || p.ordering != 1 || !reflect.DeepEqual(replay, proposal) {
				t.Fatal("reconstruction reordered", err, p.ordering)
			}
			routes := []CodingLedgerRoute{}
			for _, role := range []string{"planner", "executor", "reviewer"} {
				routes = append(routes, CodingLedgerRoute{Role: role, ConfigurationRevision: 1, Provider: "codex", Model: "fixture-model", ReasoningEffort: "medium", ServiceTier: "default"})
			}
			created, err := repo.CreateCodingLedger(ctx, CreateCodingLedgerCommand{JobID: initial.Job.ID, OperationID: turn.OperationID, OwnerTurnID: turn.OwnerTurnID, OwnerTurnText: turn.Text, Ledger: proposal.Ledger, OwnerTurnDisposition: proposal.OwnerTurnDisposition, Routes: routes})
			if err != nil {
				t.Fatal(err)
			}
			if err := c.PublishPlanningLedger(ctx, turn, created.Ledger); err != nil {
				t.Fatal(err)
			}
			if err := c.PublishPlanningLedger(ctx, turn, created.Ledger); err != nil {
				t.Fatal(err)
			}
			var text string
			if err := pool.QueryRow(ctx, `SELECT content FROM ai_channel_messages WHERE channel_id=$1 AND role='assistant'`, channel.ID).Scan(&text); err != nil || strings.Index(text, "a. Perform") > strings.Index(text, "z. Perform") {
				t.Fatal("published wrong order", text, err)
			}
			reply := command
			reply.OperationID = recoveryOperationID(t)
			reply.Text = "Keep a first."
			reply.ExpectedJobID = &initial.Job.ID
			reply.ExpectedGeneration = &initial.Job.CurrentGeneration
			if _, err := repo.SubmitChannelSessionTurn(ctx, reply); err != nil {
				t.Fatal(err)
			}
			if err := repo.MarkCodingLedgerReviewReady(ctx, claim.Authority, created.LedgerID, created.Revision); err != ErrPendingChannelSessionMessage {
				t.Fatal("pending reply lost", err)
			}
			next, err := repo.NextChannelSessionMessage(ctx, claim.Authority)
			if err != nil || next == nil || next.OperationID != reply.OperationID {
				t.Fatal("queued reply lost", err)
			}
			rank := int64(1)
			p.operations = []model.LedgerMutationOperation{{Kind: model.LedgerMutationReorder, TargetID: "a", Rank: &rank}}
			replyTurn := codingagent.PlanningTurn{OperationID: string(reply.OperationID), OwnerTurnID: string(reply.OperationID), Text: reply.Text, ObservedRevision: created.Revision}
			mutation, err := c.PlanPlanningMutation(ctx, created.Ledger, replyTurn)
			if err != nil {
				t.Fatal(err)
			}
			created, err = repo.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: reply.Text, Mutation: mutation})
			if err != nil {
				t.Fatal(err)
			}
			if err := c.PublishPlanningLedger(ctx, replyTurn, created.Ledger); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ResolvePlanningSessionMessage(ctx, claim.Authority, reply.OperationID); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ResolvePlanningSessionMessage(ctx, claim.Authority, reply.OperationID); err != nil {
				t.Fatal(err)
			}
			if p.ordering != 1 {
				t.Fatal("queued owner reorder invoked ordering")
			}
			operations := []model.LedgerMutationOperation{}
			for _, id := range []model.LedgerItemID{"a", "z"} {
				operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReview, TargetID: id, ReviewState: model.LedgerReviewAgreed})
			}
			if ties {
				rank := int64(1)
				operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReorder, TargetID: "z", Rank: &rank})
			}
			agreed, err := repo.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Agree to the displayed order.", Mutation: model.LedgerMutation{OperationID: "agree", OwnerTurnID: "owner-agree", BaseRevision: created.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"a", "z"}), Operations: operations}})
			if err != nil {
				t.Fatal(err)
			}
			if err := c.PublishPlanningLedger(ctx, codingagent.PlanningTurn{OperationID: "stale", Text: "stale"}, created.Ledger); err == nil {
				t.Fatal("published stale ledger")
			}
			journal, err := c.Evidence.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			writeOrderingEvidence(t, fmt.Sprintf("planning-ties-%v", ties), map[string]any{"fixture": "scripted planning/ordering; actual PostgreSQL", "created": created, "agreed": agreed, "journal": journal, "ordering_calls": p.ordering})
			if err := repo.MarkCodingLedgerReviewReady(ctx, claim.Authority, agreed.LedgerID, agreed.Revision); err != nil {
				t.Fatal(err)
			}
			frozen, err := repo.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: agreed.LedgerID, Revision: agreed.Revision, OperationID: "freeze", OwnerTurnID: "owner-freeze", OwnerTurnText: "Start the displayed order."})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CompleteCodingLedgerReview(ctx, initial.Job.ID, frozen.LedgerID, frozen.Revision); err != nil {
				t.Fatal(err)
			}
			executor, err := repo.ClaimNextStep(ctx, "proposal-executor")
			if err != nil || executor == nil {
				t.Fatal(err)
			}
			// Freeze inserts its rank/ID-sorted persisted projection, so fairness ties
			// already agree with display order. Do not mutate immutable queue identities.
			var first string
			if err := pool.QueryRow(ctx, `SELECT task_item_id FROM coding_agent_tasks WHERE ledger_id=$1 ORDER BY fairness_key LIMIT 1`, frozen.LedgerID).Scan(&first); err != nil || first != "a" {
				t.Fatal("freeze tie ordering", first, err)
			}

			runtime := &recoveryRuntime{mode: "ok", effects: map[codingagent.Role]int{}, parses: map[codingagent.Role]int{}}
			restarted := *repo
			for _, id := range []string{"a", "z"} {
				task, err := restarted.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
				if err != nil || task == nil || task.ID != id {
					t.Fatalf("selection %+v err%v want%s", task, err, id)
				}
				execution := codingagent.Controller{JobID: executor.Job.ID, Generation: executor.Job.CurrentGeneration, Root: root, Configuration: repo.codingAgents, Environment: runtime, Evidence: repo.CodingAgentEvidence(executor.Authority), Providers: map[string]codingagent.Provider{"codex": runtime}}
				if err := execution.RunTask(ctx, *task); err != nil {
					t.Fatal(err)
				}
				if err := repo.AcceptCodingLedgerTask(ctx, executor.Authority, task.ID); err != nil {
					t.Fatal(err)
				}
				if err := execution.RunTask(ctx, *task); err != nil {
					t.Fatal(err)
				}
			}
			if task, err := repo.NextIncompleteCodingLedgerTask(ctx, executor.Authority); err != nil || task != nil {
				t.Fatal("accepted task replay", task, err)
			}
			// Owner continuation of unchanged accepted tasks retains their persisted order.
			replanned, err := repo.ReplanJob(ctx, ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: initial.Job.ID, Feedback: "Retain exact accepted tasks.", ObservedLedgerRevision: frozen.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity})
			if err != nil {
				t.Fatal(err)
			}
			planner, err := repo.ClaimNextStep(ctx, "carry-planner")
			if err != nil || planner == nil {
				t.Fatal(err)
			}
			carry, err := repo.ProposeUnchangedCodingLedger(ctx, ProposeUnchangedCodingLedgerCommand{LedgerID: frozen.LedgerID, Revision: frozen.Revision, OperationID: "carry", OwnerTurnID: "owner-carry", OwnerTurnText: "Retain exact accepted tasks."})
			if err != nil {
				t.Fatal(err)
			}
			if carry.Ledger.Tasks[0].ID != "a" || carry.Ledger.Tasks[1].ID != "z" {
				t.Fatal("continuation lost order")
			}
			if err := repo.MarkCodingLedgerReviewReady(ctx, planner.Authority, carry.LedgerID, carry.Revision); err != nil {
				t.Fatal(err)
			}
			carried, err := repo.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: carry.LedgerID, Revision: carry.Revision, OperationID: "carry-freeze", OwnerTurnID: "owner-carry-freeze", OwnerTurnText: "Start unchanged accepted tasks."})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CompleteCodingLedgerReview(ctx, replanned.Job.ID, carried.LedgerID, carried.Revision); err != nil {
				t.Fatal(err)
			}
			continued, err := repo.ClaimNextStep(ctx, "carry-executor")
			if err != nil || continued == nil {
				t.Fatal(err)
			}
			if task, err := repo.NextIncompleteCodingLedgerTask(ctx, continued.Authority); err != nil || task != nil {
				t.Fatal("accepted carry selected work", task, err)
			}
			if p.ordering != 1 || runtime.effects[codingagent.Execute] != 2 || runtime.effects[codingagent.Review] != 2 {
				t.Fatal("unexpected inference/work", p.ordering, runtime.effects)
			}
			writeOrderingEvidence(t, fmt.Sprintf("accepted-ties-%v", ties), map[string]any{"fixture": "scripted native work; real controller/PostgreSQL", "frozen": frozen, "carried": carried, "ordering_calls": p.ordering, "executions": runtime.effects[codingagent.Execute], "independent_reviews": runtime.effects[codingagent.Review]})
			t.Logf("persisted/proposed/frozen/selected a,z; tie=%v; ordering calls1; executions2 independent reviews2; accepted replay0", ties)
		})
	}
}

func (p *proposalOrderingProvider) OrderingClient(context.Context) (llm.ExactStationClient, func(), error) {
	return p, func() {}, nil
}

func writeOrderingEvidence(t *testing.T, name string, value any) {
	t.Helper()
	if dir := os.Getenv("OMNI_ORDERING_EVIDENCE"); dir != "" {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
