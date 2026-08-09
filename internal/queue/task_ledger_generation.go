package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func validateTaskCommandGenerationTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	command taskstate.Command,
) error {
	stepID, err := taskCommandStepTarget(command)
	if err != nil || stepID == nil {
		return err
	}
	if *stepID <= 0 {
		return fmt.Errorf("%w: task command step ID must be positive", taskstate.ErrInvalidCommand)
	}
	var generation, currentGeneration int64
	var supersededAt *int64
	err = tx.QueryRow(ctx, `
		SELECT steps.generation, steps.superseded_at_generation, jobs.current_generation
		FROM job_steps AS steps
		JOIN jobs ON jobs.id = steps.job_id
		WHERE steps.id = $1 AND steps.job_id = $2
	`, *stepID, jobID).Scan(&generation, &supersededAt, &currentGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf(
			"%w: task command step %d does not belong to job %d",
			taskstate.ErrInvalidCommand, *stepID, jobID,
		)
	}
	if err != nil {
		return err
	}
	if supersededAt != nil || generation != currentGeneration {
		return fmt.Errorf(
			"%w: task command step %d generation %d is not job %d generation %d",
			ErrStaleJobGeneration, *stepID, generation, jobID, currentGeneration,
		)
	}
	return nil
}

func taskCommandStepTarget(command taskstate.Command) (*int64, error) {
	switch typed := command.(type) {
	case taskstate.AddNodeCommand:
		return typed.CreatedStepID, nil
	case *taskstate.AddNodeCommand:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil add-node command", taskstate.ErrInvalidCommand)
		}
		return typed.CreatedStepID, nil
	case taskstate.AddEntryCommand:
		return typed.CreatedStepID, nil
	case *taskstate.AddEntryCommand:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil add-entry command", taskstate.ErrInvalidCommand)
		}
		return typed.CreatedStepID, nil
	case taskstate.AcceptDecisionCommand:
		return typed.CreatedStepID, nil
	case *taskstate.AcceptDecisionCommand:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil accept-decision command", taskstate.ErrInvalidCommand)
		}
		return typed.CreatedStepID, nil
	case taskstate.AssignNodeStepCommand:
		return &typed.StepID, nil
	case *taskstate.AssignNodeStepCommand:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil assign-step command", taskstate.ErrInvalidCommand)
		}
		return &typed.StepID, nil
	case taskstate.TransitionNodeCommand:
		return typed.CompletedStepID, nil
	case *taskstate.TransitionNodeCommand:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil transition-node command", taskstate.ErrInvalidCommand)
		}
		return typed.CompletedStepID, nil
	case taskstate.CloseLedgerCommand:
		return typed.StepID, nil
	case *taskstate.CloseLedgerCommand:
		if typed == nil {
			return nil, fmt.Errorf("%w: nil close-ledger command", taskstate.ErrInvalidCommand)
		}
		return typed.StepID, nil
	case taskstate.AddEdgeCommand, *taskstate.AddEdgeCommand,
		taskstate.RejectEntryCommand, *taskstate.RejectEntryCommand,
		taskstate.ResolveEntryCommand, *taskstate.ResolveEntryCommand,
		taskstate.SupersedeEntryCommand, *taskstate.SupersedeEntryCommand,
		taskstate.PromoteReadyNodesCommand, *taskstate.PromoteReadyNodesCommand:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: unregistered task command type %T", taskstate.ErrInvalidCommand, command)
	}
}
