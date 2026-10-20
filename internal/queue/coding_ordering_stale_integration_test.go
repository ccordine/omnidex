package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestProposalOrderingStaleSemanticResultPreservesNewerOwnerOrder(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Keep the existing owner task.")
	job, err := repo.CurrentJobDetails(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReplanJob(ctx, ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: job.Job.ID, Feedback: "Add two new tasks.", ObservedLedgerRevision: fixture.created.Revision, WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(ctx, "stale-order-planner")
	if err != nil || claim == nil {
		t.Fatal(err)
	}
	current, _, err := repo.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := &proposalOrderingProvider{}
	for _, id := range []string{"c", "b"} {
		task := proposalTask(id, 10)
		p.operations = append(p.operations, model.LedgerMutationOperation{Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{Task: &task}})
	}
	var newer CodingLedgerRevision
	p.duringOrder = func() {
		rank := int64(42)
		newer, err = repo.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{LedgerID: current.LedgerID, OwnerTurnText: "Keep my newer explicit order.", Mutation: model.LedgerMutation{OperationID: "newer-owner", OwnerTurnID: "newer-owner-turn", BaseRevision: current.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	c := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Job.CurrentGeneration, Root: command.WorkspaceRoot, Configuration: repo.codingAgents, Environment: p, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": p}}
	turn := codingagent.PlanningTurn{OperationID: "slow-order", OwnerTurnID: "slow-order-turn", Text: "Add two new tasks.", ObservedRevision: current.Revision}
	mutation, err := c.PlanPlanningMutation(ctx, current.Ledger, turn)
	if err != nil {
		t.Fatal(err)
	}
	if p.ordering != 1 {
		t.Fatal("new unordered additions did not use one call")
	}
	if _, err := repo.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{LedgerID: current.LedgerID, OwnerTurnText: turn.Text, Mutation: mutation}); !errors.Is(err, ErrCodingLedgerRevision) {
		t.Fatal("stale ordering overwrote owner revision", err)
	}
	retained, _, err := repo.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
	if err != nil || retained.Revision != newer.Revision || len(retained.Ledger.Tasks) != 1 || retained.Ledger.Tasks[0].Rank != 42 {
		t.Fatal("new owner order lost", retained, err)
	}
	if err := c.PublishPlanningLedger(ctx, turn, current.Ledger); !errors.Is(err, ErrCodingLedgerRevision) {
		t.Fatal("stale proposal published", err)
	}
	journal, err := c.Evidence.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	writeOrderingEvidence(t, "stale-ordering", map[string]any{"fixture": "scripted ordering; actual PostgreSQL", "newer_owner": newer, "retained": retained, "rejected_mutation": mutation, "journal": journal, "ordering_calls": p.ordering})
	t.Log("one ordering call; stale result and stale publication rejected; newer owner rank42 preserved; no new tasks committed")
}
