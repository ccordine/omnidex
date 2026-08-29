package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func requireCanonicalRootActivationEventTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	root taskstate.Node,
) error {
	if root.Status != taskstate.NodeActive || root.UpdatedVersion == 0 {
		return fmt.Errorf(
			"%w: job %d root is not valid active authority",
			taskstate.ErrInvalidState, header.Owner.JobID,
		)
	}
	var generation int64
	err := tx.QueryRow(ctx, `
		SELECT job_generation FROM task_events
		WHERE ledger_id=$1 AND ledger_version=$2
	`, header.ID, int64(root.UpdatedVersion)).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf(
			"%w: job %d active root has no immutable activation version",
			taskstate.ErrInvalidState, header.Owner.JobID,
		)
	}
	if err != nil {
		return fmt.Errorf("read root activation generation for job %d: %w", header.Owner.JobID, err)
	}
	return requireCanonicalRootTransitionEventTx(
		ctx, tx, header, generation, 0, taskstate.NodeActive, "", "",
	)
}

func requireCanceledTaskAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	reason string,
) error {
	if job.Status != model.JobStatusCanceled || job.CompletedAt == nil {
		return fmt.Errorf(
			"%w: job %d does not contain complete canceled authority",
			taskstate.ErrInvalidState, job.ID,
		)
	}
	header, root, err := loadInitialTaskRootTx(ctx, tx, job.ID, job.CurrentGeneration)
	if err != nil {
		return err
	}
	if header.JobStatus != model.JobStatusCanceled ||
		header.Status != taskstate.LedgerCanceled ||
		root.Status != taskstate.NodeCanceled || root.StatusReason != reason {
		return fmt.Errorf(
			"%w: canceled job %d disagrees with its canonical task state",
			taskstate.ErrInvalidState, job.ID,
		)
	}
	if err := requireCanonicalRootTransitionEventTx(
		ctx, tx, header, job.CurrentGeneration, 0,
		taskstate.NodeCanceled, "", reason,
	); err != nil {
		return err
	}
	return requireCanonicalLedgerCloseEventTx(
		ctx, tx, header, job.CurrentGeneration,
		taskstate.LedgerCanceled, nil, reason,
	)
}

func requireCanonicalRootTransitionEventTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	generation, stepID int64,
	to taskstate.NodeStatus,
	proofContent, reason string,
) error {
	operation := "root-" + string(to)
	if to == taskstate.NodeActive {
		operation = "activate-root"
	}
	commandID, err := initialLifecycleCommandID(header.Owner.JobID, generation, stepID, operation)
	if err != nil {
		return err
	}
	version, err := canonicalTaskEventVersionTx(ctx, tx, header, commandID)
	if err != nil {
		return err
	}
	command := taskstate.TransitionNodeCommand{
		CommandID: commandID, ExpectedVersion: version - 1,
		Actor: taskstate.AuthorityCode, NodeID: initialTaskRootNodeID,
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
		event.NodeID != initialTaskRootNodeID || event.ToStatus != to ||
		event.Reason != reason {
		return fmt.Errorf(
			"%w: job %d is missing its canonical %q root event",
			taskstate.ErrInvalidState, header.Owner.JobID, to,
		)
	}
	if to == taskstate.NodeActive && event.FromStatus != taskstate.NodeReady {
		return fmt.Errorf(
			"%w: job %d canonical root activation did not start from ready",
			taskstate.ErrInvalidState, header.Owner.JobID,
		)
	}
	if to == taskstate.NodeDone {
		if !sameTaskEventStepID(event.StepID, &stepID) ||
			len(event.VerificationRefs) != 1 ||
			event.VerificationRefs[0] != command.VerificationRefs[0] {
			return fmt.Errorf(
				"%w: job %d canonical root completion proof is inconsistent",
				taskstate.ErrInvalidState, header.Owner.JobID,
			)
		}
	} else if event.StepID != nil || len(event.VerificationRefs) != 0 {
		return fmt.Errorf(
			"%w: job %d canonical root transition contains forbidden proof state",
			taskstate.ErrInvalidState, header.Owner.JobID,
		)
	}
	return nil
}

func requireCanonicalLedgerCloseEventTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	generation int64,
	status taskstate.LedgerStatus,
	stepID *int64,
	reason string,
) error {
	commandID, err := terminalTaskCommandID(header.Owner.JobID, generation, status)
	if err != nil {
		return err
	}
	version, err := canonicalTaskEventVersionTx(ctx, tx, header, commandID)
	if err != nil {
		return err
	}
	command := taskstate.CloseLedgerCommand{
		CommandID: commandID, ExpectedVersion: version - 1,
		Actor: taskstate.AuthorityCode, Status: status, StepID: stepID, Reason: reason,
	}
	descriptor, err := taskstate.DescribeCommand(command)
	if err != nil {
		return err
	}
	event, found, err := loadTaskEventByCommandTx(ctx, tx, header, generation, descriptor)
	if err != nil {
		return err
	}
	if !found || event.Kind != taskstate.EventLedgerClosed ||
		event.LedgerStatus != status || event.Reason != reason ||
		!sameTaskEventStepID(event.StepID, stepID) {
		return fmt.Errorf(
			"%w: job %d is missing its canonical %q ledger event",
			taskstate.ErrInvalidState, header.Owner.JobID, status,
		)
	}
	return nil
}

func canonicalTaskEventVersionTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	commandID taskstate.CommandID,
) (uint64, error) {
	var version int64
	err := tx.QueryRow(ctx, `
		SELECT ledger_version FROM task_events
		WHERE ledger_id=$1 AND command_id=$2
	`, header.ID, commandID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf(
			"%w: canonical task event %q is missing",
			taskstate.ErrInvalidState, commandID,
		)
	}
	if err != nil {
		return 0, fmt.Errorf("read canonical task event %q: %w", commandID, err)
	}
	if version <= 0 || uint64(version) > header.Version {
		return 0, fmt.Errorf(
			"%w: canonical task event %q has invalid version %d",
			taskstate.ErrInvalidState, commandID, version,
		)
	}
	return uint64(version), nil
}

func sameTaskEventStepID(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
