package queue

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func closeCognitionTerminalWorkingSetTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	jobID, generation int64,
	episodeID cognition.EpisodeID,
	outcome CognitionEpisodeStatus,
	revision cognition.WorldRevision,
	graph cognition.ObligationGraphSnapshot,
	worker *model.StepAttemptAuthority,
	retirement *cognitionLifecycleRetirement,
) (workingset.Event, error) {
	if (worker == nil) == (retirement == nil) {
		return workingset.Event{}, fmt.Errorf("cognition terminal working set requires one exact authority")
	}
	set, err := loadWorkingSetSnapshotTx(ctx, tx, header, generation, true)
	if err != nil {
		return workingset.Event{}, err
	}
	authorityID := cognitionTerminalAuthorityWorker
	if retirement != nil {
		if err := retirement.Validate(); err != nil || retirement.JobID != jobID ||
			retirement.JobGeneration != generation || retirement.EpisodeID != episodeID ||
			retirement.ExpectedRevision != revision || retirement.GraphSHA256 != graph.SHA256 {
			return workingset.Event{}, fmt.Errorf("%w: lifecycle working-set authority changed", ErrCognitionConflict)
		}
		authorityID = retirement.ID
	}
	workingID, err := workingset.NewCommandID(
		"omnidex.cognition-terminal.v2", string(episodeID), string(outcome), authorityID,
		strconv.FormatUint(revision.Number, 10), revision.SHA256, graph.SHA256,
	)
	if err != nil {
		return workingset.Event{}, err
	}
	command := workingset.CloseScopeCommand{
		CommandID: workingID, ExpectedVersion: set.Version, Actor: taskstate.AuthorityCode,
		Scope:  workingset.Scope{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(graph.RootID)},
		Reason: "Cognition episode " + string(outcome) + ".",
	}
	descriptor, err := workingset.DescribeCommand(command)
	if err != nil {
		return workingset.Event{}, err
	}
	if worker != nil {
		return applyWorkingSetCommandTx(ctx, tx, *worker, command, descriptor)
	}
	if err := requireCognitionLifecycleRegistryTx(ctx, tx, *retirement); err != nil {
		return workingset.Event{}, err
	}
	return applyAuthorizedWorkingSetCommandTx(ctx, tx, jobID, generation, command, descriptor)
}

func requireCognitionLifecycleRegistryTx(
	ctx context.Context,
	tx pgx.Tx,
	retirement cognitionLifecycleRetirement,
) error {
	var exact bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM lifecycle_operation_registry
		 WHERE operation_id=$1 AND kind=$2 AND command_sha256=$3)
	`, retirement.OperationID, retirement.OperationKind, retirement.OperationSHA256).Scan(&exact)
	if err != nil {
		return err
	}
	if !exact {
		return fmt.Errorf("%w: lifecycle operation registry authority changed", ErrLifecycleOperationConflict)
	}
	return nil
}
