package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func planCognitionBeliefRevision(
	ledger taskstate.MaterializedState,
	prepared cognitionruntime.PreparedSnapshot,
	command cognitionruntime.ReconciliationCommand,
) (*cognitionstate.BeliefRevisionMaterialization, error) {
	materialization, found, err := cognitionstate.PlanBeliefRevision(cognitionstate.ModelProposalInput{
		Ledger: ledger, ScopeNodeID: taskstate.NodeID(command.Decision.ObligationID),
		Snapshot: prepared.Snapshot, Decision: command.Decision, ActionSchema: command.ActionSchema,
	})
	if err != nil {
		return nil, fmt.Errorf("plan cognition belief revision: %w", err)
	}
	if !found {
		return nil, nil
	}
	return &materialization, nil
}

func applyCognitionBeliefRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	before taskstate.MaterializedState,
	materialization cognitionstate.BeliefRevisionMaterialization,
) (taskLedgerHeader, taskstate.MaterializedState, error) {
	expected, err := cognitionstate.ApplyBeliefRevision(before, materialization)
	if err != nil {
		return taskLedgerHeader{}, taskstate.MaterializedState{}, err
	}
	if _, err := applyQueueOwnedTaskCommandTx(
		ctx, tx, authority.JobID, authority.Generation, materialization.Rejection,
	); err != nil {
		return taskLedgerHeader{}, taskstate.MaterializedState{},
			fmt.Errorf("persist cognition belief revision: %w", err)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, false)
	if err != nil {
		return taskLedgerHeader{}, taskstate.MaterializedState{}, err
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return taskLedgerHeader{}, taskstate.MaterializedState{}, err
	}
	after := ledger.MaterializedState()
	if !reflect.DeepEqual(after, expected) {
		return taskLedgerHeader{}, taskstate.MaterializedState{}, fmt.Errorf(
			"%w: persisted belief revision differs from its code-owned descriptor",
			ErrCognitionConflict,
		)
	}
	return header, after, nil
}
