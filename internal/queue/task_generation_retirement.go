package queue

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const taskGenerationRetirementSchema = "omnidex.task-generation-retirement.v1"

func supersedeCurrentCognitionObligationsTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	retiringGeneration, successorGeneration int64,
) error {
	if header.Generation != retiringGeneration || retiringGeneration <= 0 ||
		successorGeneration != retiringGeneration+1 || header.Status != taskstate.LedgerActive {
		return fmt.Errorf("%w: cognition obligation retirement authority is inconsistent", ErrInvalidJobGeneration)
	}
	rows, err := tx.Query(ctx, `
		SELECT obligations.node_id
		FROM cognition_obligations AS obligations
		JOIN task_nodes AS nodes
		  ON nodes.ledger_id=obligations.ledger_id AND nodes.id=obligations.node_id
		WHERE obligations.job_id=$1 AND obligations.ledger_id=$2
		  AND obligations.job_generation=$3
		ORDER BY obligations.node_id ASC
		FOR UPDATE OF nodes
	`, header.Owner.JobID, header.ID, retiringGeneration)
	if err != nil {
		return fmt.Errorf("lock generation %d cognition obligations for job %d: %w",
			retiringGeneration, header.Owner.JobID, err)
	}
	nodeIDs := make([]taskstate.NodeID, 0)
	for rows.Next() {
		var nodeID taskstate.NodeID
		if err := rows.Scan(&nodeID); err != nil {
			rows.Close()
			return fmt.Errorf("scan cognition obligation retirement: %w", err)
		}
		nodeIDs = append(nodeIDs, nodeID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read cognition obligation retirement set: %w", err)
	}
	rows.Close()
	if len(nodeIDs) == 0 {
		return nil
	}
	command, err := taskGenerationRetirementCommand(
		header.Owner.JobID, header.Version, retiringGeneration, successorGeneration, nodeIDs,
	)
	if err != nil {
		return err
	}
	_, err = applyQueueOwnedTaskCommandTx(
		ctx, tx, header.Owner.JobID, retiringGeneration, command,
	)
	if err != nil {
		return fmt.Errorf("supersede job %d generation %d cognition obligations: %w",
			header.Owner.JobID, retiringGeneration, err)
	}
	return nil
}

func taskGenerationRetirementCommand(
	jobID int64,
	expectedVersion uint64,
	retiringGeneration, successorGeneration int64,
	nodeIDs []taskstate.NodeID,
) (taskstate.SupersedeNodeGenerationCommand, error) {
	commandID, err := taskstate.NewCommandID(
		taskGenerationRetirementSchema,
		strconv.FormatInt(jobID, 10),
		strconv.FormatInt(retiringGeneration, 10),
		strconv.FormatInt(successorGeneration, 10),
	)
	if err != nil {
		return taskstate.SupersedeNodeGenerationCommand{}, err
	}
	reason := fmt.Sprintf(
		"Job generation %d superseded cognition obligations from generation %d.",
		successorGeneration, retiringGeneration,
	)
	return taskstate.SupersedeNodeGenerationCommand{
		CommandID: commandID, ExpectedVersion: expectedVersion,
		Actor:              taskstate.AuthorityCode,
		RetiringGeneration: retiringGeneration, SupersededAtGeneration: successorGeneration,
		NodeIDs: append([]taskstate.NodeID(nil), nodeIDs...), Reason: reason,
	}, nil
}
