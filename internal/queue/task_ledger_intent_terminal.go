package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

type acceptedIntentObjectiveAuthority struct {
	ArtifactID int64
	LedgerID   taskstate.LedgerID
	NodeID     taskstate.NodeID
}

func transitionAcceptedIntentObjectiveTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation, stepID int64,
	to taskstate.NodeStatus,
	proofContent, reason string,
) error {
	if !acceptedIntentTerminalStatus(to) {
		return fmt.Errorf("%w: accepted intent objective cannot transition to %q", taskstate.ErrInvalidCommand, to)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, true)
	if err != nil {
		return err
	}
	if header.Generation != generation || header.Status != taskstate.LedgerActive {
		return fmt.Errorf(
			"%w: accepted intent terminal transition observed generation %d with ledger generation %d status %q",
			ErrStaleJobGeneration, generation, header.Generation, header.Status,
		)
	}
	authority, found, err := loadAcceptedIntentObjectiveAuthorityTx(ctx, tx, header)
	if err != nil || !found {
		return err
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return err
	}
	objective, ok := ledger.Node(authority.NodeID)
	if !ok || objective.Kind != taskstate.NodeObjective || objective.Status != taskstate.NodeActive ||
		objective.AssignedStepID != nil {
		return fmt.Errorf("%w: accepted intent objective cannot terminalize from its persisted state", taskstate.ErrInvalidState)
	}
	commandID, err := acceptedIntentTerminalCommandID(authority, jobID, generation, stepID, to)
	if err != nil {
		return err
	}
	command := taskstate.TransitionNodeCommand{
		CommandID: commandID, ExpectedVersion: header.Version,
		Actor: taskstate.AuthorityCode, NodeID: authority.NodeID,
		To: to, Reason: reason,
	}
	if to == taskstate.NodeDone {
		if stepID <= 0 {
			return fmt.Errorf("accepted intent objective completion requires a positive terminal step")
		}
		command.CompletedStepID = &stepID
		command.VerificationRefs = []taskstate.Ref{
			stepCompletionRef(jobID, generation, stepID, proofContent),
		}
	}
	if _, err := applyQueueOwnedTaskCommandTx(ctx, tx, jobID, generation, command); err != nil {
		return fmt.Errorf("terminalize accepted intent objective: %w", err)
	}
	return nil
}

func requireAcceptedIntentObjectiveTerminalEventTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	generation, stepID int64,
	to taskstate.NodeStatus,
	proofContent, reason string,
) error {
	if !acceptedIntentTerminalStatus(to) {
		return nil
	}
	authority, found, err := loadAcceptedIntentObjectiveAuthorityTx(ctx, tx, header)
	if err != nil || !found {
		return err
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return err
	}
	objective, ok := ledger.Node(authority.NodeID)
	if !ok || objective.Kind != taskstate.NodeObjective || objective.Status != to ||
		objective.StatusReason != reason || objective.AssignedStepID != nil {
		return fmt.Errorf("%w: accepted intent objective terminal state is inconsistent", taskstate.ErrInvalidState)
	}
	commandID, err := acceptedIntentTerminalCommandID(authority, header.Owner.JobID, generation, stepID, to)
	if err != nil {
		return err
	}
	version, err := canonicalTaskEventVersionTx(ctx, tx, header, commandID)
	if err != nil {
		return err
	}
	command := taskstate.TransitionNodeCommand{
		CommandID: commandID, ExpectedVersion: version - 1,
		Actor: taskstate.AuthorityCode, NodeID: authority.NodeID,
		To: to, Reason: reason,
	}
	if to == taskstate.NodeDone {
		command.CompletedStepID = &stepID
		command.VerificationRefs = []taskstate.Ref{
			stepCompletionRef(header.Owner.JobID, generation, stepID, proofContent),
		}
	}
	descriptor, err := taskstate.DescribeCommand(command)
	if err != nil {
		return err
	}
	event, found, err := loadTaskEventByCommandTx(ctx, tx, header, generation, descriptor)
	if err != nil {
		return err
	}
	if !found || event.Kind != taskstate.EventNodeTransitioned ||
		event.NodeID != authority.NodeID || event.FromStatus != taskstate.NodeActive ||
		event.ToStatus != to || event.Reason != reason {
		return fmt.Errorf("%w: accepted intent objective terminal event is inconsistent", taskstate.ErrInvalidState)
	}
	if to == taskstate.NodeDone {
		if !sameTaskEventStepID(event.StepID, &stepID) ||
			len(event.VerificationRefs) != 1 || event.VerificationRefs[0] != command.VerificationRefs[0] {
			return fmt.Errorf("%w: accepted intent objective completion proof is inconsistent", taskstate.ErrInvalidState)
		}
	} else if event.StepID != nil || len(event.VerificationRefs) != 0 {
		return fmt.Errorf("%w: accepted intent objective failure contains forbidden proof", taskstate.ErrInvalidState)
	}
	return nil
}

func loadAcceptedIntentObjectiveAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
) (acceptedIntentObjectiveAuthority, bool, error) {
	var authority acceptedIntentObjectiveAuthority
	var ledgerID, nodeID string
	err := tx.QueryRow(ctx, `
		SELECT projection.artifact_id, projection.ledger_id, projection.objective_node_id
		FROM task_artifact_projections AS projection
		JOIN task_artifact_projection_items AS item
		  ON item.artifact_id=projection.artifact_id
		 AND item.job_id=projection.job_id
		 AND item.ledger_id=projection.ledger_id
		 AND item.item_kind='objective' AND item.ordinal=0
		 AND item.node_id=projection.objective_node_id
		WHERE projection.job_id=$1 AND projection.artifact_kind=$2
	`, header.Owner.JobID, artifacts.KindIntent).Scan(&authority.ArtifactID, &ledgerID, &nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		var intentArtifacts int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM artifacts WHERE job_id=$1 AND kind=$2
		`, header.Owner.JobID, artifacts.KindIntent).Scan(&intentArtifacts); err != nil {
			return acceptedIntentObjectiveAuthority{}, false, err
		}
		if intentArtifacts != 0 {
			return acceptedIntentObjectiveAuthority{}, false, fmt.Errorf(
				"%w: job %d has intent artifact authority without its task projection",
				taskstate.ErrInvalidState, header.Owner.JobID,
			)
		}
		return acceptedIntentObjectiveAuthority{}, false, nil
	}
	if err != nil {
		return acceptedIntentObjectiveAuthority{}, false, fmt.Errorf("load accepted intent objective authority: %w", err)
	}
	authority.LedgerID, authority.NodeID = taskstate.LedgerID(ledgerID), taskstate.NodeID(nodeID)
	if authority.ArtifactID <= 0 || authority.LedgerID != header.ID || authority.NodeID == "" {
		return acceptedIntentObjectiveAuthority{}, false, fmt.Errorf(
			"%w: job %d accepted intent objective binding is inconsistent",
			taskstate.ErrInvalidState, header.Owner.JobID,
		)
	}
	return authority, true, nil
}

func acceptedIntentTerminalCommandID(
	authority acceptedIntentObjectiveAuthority,
	jobID, generation, stepID int64,
	to taskstate.NodeStatus,
) (taskstate.CommandID, error) {
	return taskstate.NewCommandID(
		acceptedIntentProjectionSchema, strconv.FormatInt(jobID, 10),
		strconv.FormatInt(authority.ArtifactID, 10), strconv.FormatInt(generation, 10),
		strconv.FormatInt(stepID, 10), "terminal-objective-"+string(to),
	)
}

func acceptedIntentTerminalStatus(status taskstate.NodeStatus) bool {
	return status == taskstate.NodeDone || status == taskstate.NodeFailed || status == taskstate.NodeCanceled
}
