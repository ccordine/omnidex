package queue

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestRestoreAcceptedTaskRecoversExactContractAndKeepsHistory(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	job, err := repository.EnqueueCodingJobMode(t.Context(), "Preserve accepted work while changing pending order.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(t.Context(), "restore-planner")
	if err != nil || planner == nil {
		t.Fatalf("planner: %v", err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, "Implement the accepted task.")
	frozen := freezeCodingAgentPlanFixture(t, repository, job, fixture)
	executor, err := repository.ClaimNextStep(t.Context(), "restore-executor")
	if err != nil || executor == nil {
		t.Fatalf("executor: %v", err)
	}
	task, err := repository.NextIncompleteCodingLedgerTask(t.Context(), executor.Authority)
	if err != nil || task == nil {
		t.Fatalf("task: %v", err)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *task)
	root, identity := codingPlanWorkspaceAuthority(t, job)
	_, err = repository.ReplanJob(t.Context(), ReplanJobCommand{OperationID: codingPlanOperationID(t, "restore-replan", job.ID),
		JobID: job.ID, Feedback: "Reorder pending work, retaining accepted work.", ObservedLedgerRevision: frozen.Revision,
		WorkspaceRoot: root, WorkspaceIdentity: identity})
	if err != nil {
		t.Fatal(err)
	}
	original := frozen.Ledger.Tasks[0]
	changed := original
	changed.Goal = "Obsolete queued instructions"
	changed.OwnerContext = []model.TaskContext{{Text: "Obsolete fixture instruction", Provenance: original.Provenance}}
	changed.Constraints = []string{"Retain a fixture that was already removed."}
	changed.Rank = original.Rank + 9
	amended, err := repository.ApplyCodingLedgerMutation(t.Context(), ApplyCodingLedgerMutationCommand{
		LedgerID: frozen.LedgerID, OwnerTurnText: "Obsolete queued edit", Mutation: model.LedgerMutation{
			OperationID: "restore-stale-edit", OwnerTurnID: "restore-stale-turn", BaseRevision: frozen.Revision,
			OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{original.ID}),
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: original.ID, Item: &model.LedgerMutationItem{Task: &changed}, Rationale: "old instruction"}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	// A mistaken planning reply may already have marked the edited draft
	// agreed. Restoring must respect the database's content-change guard.
	amended, err = repository.ApplyCodingLedgerControl(t.Context(), ApplyCodingLedgerMutationCommand{LedgerID: frozen.LedgerID,
		OwnerTurnText: "Mark the edited draft agreed.", Mutation: model.LedgerMutation{
			OperationID: "restore-stale-review", OwnerTurnID: "restore-stale-review-owner", BaseRevision: amended.Revision,
			OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{original.ID}),
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: original.ID, ReviewState: model.LedgerReviewAgreed, Rationale: "draft review"}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	command := ApplyCodingLedgerMutationCommand{LedgerID: frozen.LedgerID, OwnerTurnText: "Restore the saved accepted task, preserving its current order.", Mutation: model.LedgerMutation{
		OperationID: "restore-accepted", OwnerTurnID: "restore-owner", BaseRevision: amended.Revision,
		OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{original.ID}),
		Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationRestore, TargetID: original.ID, Rationale: "owner restores accepted contract"}},
	}}
	restored, err := repository.ApplyCodingLedgerControl(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	want := original
	want.Rank = changed.Rank
	want.ReviewState = model.LedgerReviewProposed
	if !reflect.DeepEqual(restored.Ledger.Tasks[0], want) {
		t.Fatalf("restore changed accepted contract: got=%+v want=%+v", restored.Ledger.Tasks[0], want)
	}
	prior, err := repository.ReadCodingLedger(t.Context(), frozen.LedgerID, CodingLedgerReadOptions{Revision: amended.Revision})
	if err != nil || prior.Ledger.Tasks[0].Goal != changed.Goal {
		t.Fatalf("unapproved history was lost: %v", err)
	}
	replay, err := repository.ApplyCodingLedgerControl(t.Context(), command)
	if err != nil || replay.Applied || replay.Revision != restored.Revision {
		t.Fatalf("restore replay: %+v %v", replay, err)
	}
	restored, err = repository.ApplyCodingLedgerControl(t.Context(), ApplyCodingLedgerMutationCommand{LedgerID: frozen.LedgerID,
		OwnerTurnText: "Approve the restored proposal.", Mutation: model.LedgerMutation{
			OperationID: "restore-review", OwnerTurnID: "restore-review-owner", BaseRevision: restored.Revision,
			OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{original.ID}),
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: original.ID, ReviewState: model.LedgerReviewAgreed, Rationale: "approve restored draft"}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	refrozen, err := repository.FreezeCodingLedger(t.Context(), FreezeCodingLedgerCommand{LedgerID: restored.LedgerID,
		Revision: restored.Revision, OperationID: "restore-refreeze", OwnerTurnID: "restore-start", OwnerTurnText: "Start the remaining work."})
	if err != nil {
		t.Fatal(err)
	}
	var state string
	if err := pool.QueryRow(t.Context(), `SELECT state FROM coding_agent_tasks WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3`, refrozen.LedgerID, refrozen.Revision, original.ID).Scan(&state); err != nil || state != "accepted" {
		t.Fatalf("restored accepted task reopened: %s %v", state, err)
	}
}
