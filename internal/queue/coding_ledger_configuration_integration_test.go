package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingLedgerChangedRoutesCanBeRefrozenWithExactConfigurationAuthority(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Refreeze changed routes.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ids := make([]model.LedgerItemID, 0)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "route-refreeze-create", OwnerTurnID: "route-second-freeze-turn:sign_off",
		OwnerTurnText: "Refreeze changed routes.", Ledger: ledger,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(ids),
		Routes:               repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	operations := make([]model.LedgerMutationOperation, 0, len(ids))
	for _, id := range ids {
		operations = append(operations, model.LedgerMutationOperation{
			Kind: model.LedgerMutationReview, TargetID: id,
			ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed",
		})
	}
	agreed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Agree to the initial ledger.",
		Mutation: model.LedgerMutation{
			OperationID: "route-refreeze-agree", BaseRevision: created.Revision,
			OwnerTurnID:          "route-refreeze-agree-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn(ids),
			Operations:           operations,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstFrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: agreed.Revision, OperationID: "route-first-freeze",
		OwnerTurnID: "route-first-freeze-turn", OwnerTurnText: "Freeze the initial routes.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE coding_ledgers
		SET state='revising',frozen_revision=NULL,updated_at=clock_timestamp()
		WHERE id=$1 AND current_revision=$2
	`, created.LedgerID, firstFrozen.Revision); err != nil {
		t.Fatal(err)
	}
	replacement := repositoryLedgerRoutes(firstFrozen.Revision + 1)
	for index := range replacement {
		replacement[index].ReasoningEffort = "xhigh"
	}
	changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Use the replacement fast routes.",
		Mutation: model.LedgerMutation{
			OperationID: "route-replacement", BaseRevision: firstFrozen.Revision,
			OwnerTurnID: "route-replacement-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn(
				[]model.LedgerItemID{"task"}),

			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReview, TargetID: "task",
				ReviewState: model.LedgerReviewAgreed, Rationale: "approve replacement route authority",
			}},
		},
		Routes: replacement,
	})
	if err != nil {
		t.Fatal(err)
	}
	refrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: changed.Revision, OperationID: "route-second-freeze",
		OwnerTurnID: "route-second-freeze-turn", OwnerTurnText: "Freeze the replacement routes.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if refrozen.State != "frozen" || refrozen.BaseRevision != changed.Revision {
		t.Fatalf("replacement route freeze = %#v", refrozen)
	}
	var action string
	var represented bool
	if err := pool.QueryRow(ctx, `
		SELECT turn_record.action,
		       disposition.statements @> '[{"kind":"represented"}]'::jsonb
		FROM coding_ledger_owner_turns turn_record
		JOIN coding_ledger_owner_turn_dispositions disposition
		  USING (ledger_id,owner_turn_id)
		WHERE turn_record.ledger_id=$1 AND turn_record.owner_turn_id=$2
	`, created.LedgerID, "route-second-freeze-turn").Scan(&action, &represented); err != nil {
		t.Fatal(err)
	}
	if action != "execution_configuration" || !represented {
		t.Fatalf("replacement route authority = %s represented=%t", action, represented)
	}
}

func TestAgentsReplanCarriesOnlyTheExactAcceptedTaskContract(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Preserve exact accepted work across replanning.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "accepted-carry-planner")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("claim planning step: %#v, %v", planner, err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, "Preserve the exact accepted task.")
	firstFrozen := freezeCodingAgentPlanFixture(t, repository, job, fixture)
	executor, err := repository.ClaimNextStep(ctx, "accepted-carry-executor")
	if err != nil || executor == nil || executor.Step.Action != "v3_coding" {
		t.Fatalf("claim execution step: %#v, %v", executor, err)
	}
	task, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || task == nil {
		t.Fatalf("select first task: %#v, %v", task, err)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *task)

	root, identity := codingPlanWorkspaceAuthority(t, job)
	replanned, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "accepted-carry-replan", job.ID),
		JobID:       job.ID, Feedback: "Keep the exact approved task contract.",
		ObservedLedgerRevision: firstFrozen.Revision,
		WorkspaceRoot:          root, WorkspaceIdentity: identity,
	})
	if err != nil || !replanned.Applied {
		t.Fatalf("replan accepted task: %#v, %v", replanned, err)
	}
	var state string
	var frozenRevision *int64
	if err := pool.QueryRow(ctx, `
		SELECT state,frozen_revision FROM coding_ledgers WHERE id=$1
	`, firstFrozen.LedgerID).Scan(&state, &frozenRevision); err != nil {
		t.Fatal(err)
	}
	if state != "revising" || frozenRevision != nil {
		t.Fatalf("ledger after replan = %s/%v", state, frozenRevision)
	}

	current, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: firstFrozen.LedgerID, OwnerTurnText: "Keep the exact approved task contract.",
		Mutation: model.LedgerMutation{
			OperationID: "accepted-carry-confirm", BaseRevision: firstFrozen.Revision,
			OwnerTurnID: "accepted-carry-confirm-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn(
				[]model.LedgerItemID{model.LedgerItemID(task.ID)}),

			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReview, TargetID: model.LedgerItemID(task.ID),
				ReviewState: model.LedgerReviewAgreed, Rationale: "the exact contract remains approved",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	refrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: current.LedgerID, Revision: current.Revision,
		OperationID: "accepted-carry-refreeze", OwnerTurnID: "accepted-carry-refreeze-turn",
		OwnerTurnText: "Freeze the unchanged approved task contract.",
	})
	if err != nil {
		t.Fatal(err)
	}
	var carriedState string
	if err := pool.QueryRow(ctx, `
		SELECT state FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3
	`, refrozen.LedgerID, refrozen.Revision, task.ID).Scan(&carriedState); err != nil {
		t.Fatal(err)
	}
	if carriedState != "accepted" {
		t.Fatalf("unchanged accepted task reopened as %q", carriedState)
	}

	changedReplan, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "accepted-contract-change-replan", job.ID),
		JobID:       job.ID, Feedback: "Replace the approved task with a materially changed contract.",
		ObservedLedgerRevision: refrozen.Revision,
		WorkspaceRoot:          root, WorkspaceIdentity: identity,
	})
	if err != nil || !changedReplan.Applied {
		t.Fatalf("replan changed task: %#v, %v", changedReplan, err)
	}
	amendedTask := refrozen.Ledger.Tasks[0]
	amendedTask.Goal = "Implement the materially changed task contract."
	amendedTask.ReviewState = model.LedgerReviewProposed
	changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: refrozen.LedgerID, OwnerTurnText: "Replace the approved task with a materially changed contract.",
		Mutation: model.LedgerMutation{
			OperationID: "accepted-contract-change", BaseRevision: refrozen.Revision,
			OwnerTurnID: "accepted-contract-change-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn(
				[]model.LedgerItemID{amendedTask.ID}),

			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationAmend, TargetID: amendedTask.ID,
				Item: &model.LedgerMutationItem{Task: &amendedTask}, Rationale: "the task contract changed",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	agreedChange, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: changed.LedgerID, OwnerTurnText: "Approve the changed task contract.",
		Mutation: model.LedgerMutation{
			OperationID: "accepted-contract-change-agree", BaseRevision: changed.Revision,
			OwnerTurnID: "accepted-contract-change-agree-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn(
				[]model.LedgerItemID{amendedTask.ID}),

			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReview, TargetID: amendedTask.ID,
				ReviewState: model.LedgerReviewAgreed, Rationale: "approve the changed contract",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	changedFrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: agreedChange.LedgerID, Revision: agreedChange.Revision,
		OperationID: "accepted-contract-change-freeze", OwnerTurnID: "accepted-contract-change-freeze-turn",
		OwnerTurnText: "Freeze the changed approved task contract.",
	})
	if err != nil {
		t.Fatal(err)
	}
	var changedState string
	if err := pool.QueryRow(ctx, `
		SELECT state FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3
	`, changedFrozen.LedgerID, changedFrozen.Revision, task.ID).Scan(&changedState); err != nil {
		t.Fatal(err)
	}
	if changedState != "ready" {
		t.Fatalf("changed accepted task did not reopen as ready: %q", changedState)
	}

	cancelled, err := repository.CancelJob(ctx, CancelJobCommand{
		OperationID: codingPlanOperationID(t, "accepted-carry-cancel", job.ID),
		JobID:       job.ID, Reason: "Stop the remaining job without erasing accepted work.",
		WorkspaceRoot: root, WorkspaceIdentity: identity,
	})
	if err != nil || !cancelled.Applied {
		t.Fatalf("cancel accepted ledger: %#v, %v", cancelled, err)
	}
	var ledgerState, preservedState string
	if err := pool.QueryRow(ctx, `SELECT state FROM coding_ledgers WHERE id=$1`, refrozen.LedgerID).Scan(&ledgerState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT state FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3
	`, refrozen.LedgerID, refrozen.Revision, task.ID).Scan(&preservedState); err != nil {
		t.Fatal(err)
	}
	var cancelledState string
	if err := pool.QueryRow(ctx, `
		SELECT state FROM coding_agent_tasks
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3
	`, changedFrozen.LedgerID, changedFrozen.Revision, task.ID).Scan(&cancelledState); err != nil {
		t.Fatal(err)
	}
	if ledgerState != "cancelled" || preservedState != "accepted" || cancelledState != "cancelled" {
		t.Fatalf("cancellation states: ledger=%s accepted_history=%s unfinished=%s", ledgerState, preservedState, cancelledState)
	}
}
