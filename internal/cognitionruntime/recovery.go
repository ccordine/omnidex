package cognitionruntime

import (
	"context"
	"errors"
	"fmt"
)

func (runtime *Runtime) recover(
	ctx context.Context,
	binding Binding,
) (StepResult, bool, error) {
	record, err := runtime.actions.Unresolved(ctx, binding)
	if err != nil {
		return StepResult{}, false, fmt.Errorf("load unresolved cognition action: %w", err)
	}
	if record == nil {
		return runtime.recoverAcceptedOrTerminal(ctx, binding)
	}
	partial := StepResult{
		Binding: binding, Revision: record.ExpectedRevision, ActionID: record.Action.ID,
		RecoveredAction: true,
	}
	if err := record.ValidateFor(binding); err != nil {
		return partial, true, err
	}
	if record.Status != ActionPrepared && record.Status != ActionDispatched {
		return partial, true, fmt.Errorf("%w: unresolved lookup returned %q", ErrInvalidJournalState, record.Status)
	}
	result, err := runtime.execute(ctx, binding, *record, record.Status == ActionPrepared, false, false, true)
	return result, true, err
}

func (runtime *Runtime) recoverAcceptedOrTerminal(
	ctx context.Context,
	binding Binding,
) (StepResult, bool, error) {
	recovery, err := runtime.accepted.RecoverAccepted(ctx, binding)
	if err != nil {
		return StepResult{}, false, fmt.Errorf("load accepted cognition decision: %w", err)
	}
	if recovery != nil {
		result, err := runtime.recoverAcceptedDecision(ctx, binding, *recovery)
		return result, true, err
	}
	return runtime.recoverTerminalProgress(ctx, binding)
}

func (runtime *Runtime) recoverTerminalProgress(
	ctx context.Context,
	binding Binding,
) (StepResult, bool, error) {
	progress, err := runtime.episodes.TerminalProgress(ctx, binding)
	if err != nil {
		return StepResult{}, false, fmt.Errorf("load terminal cognition progress: %w", err)
	}
	if progress == nil {
		return runtime.recoverTerminalPolicyOutcome(ctx, binding)
	}
	if progress.State == ProgressCanceled {
		result, err := recoveredCanceledProgress(binding, *progress)
		result.RecoveredProgress = true
		return result, true, err
	}
	if progress.State != ProgressCompleted && progress.State != ProgressFailed {
		return StepResult{}, true, fmt.Errorf(
			"%w: terminal-progress lookup returned %q", ErrInvalidProgress, progress.State,
		)
	}
	result, err := runtime.sealProgress(ctx, binding, *progress)
	result.RecoveredProgress = true
	return result, true, err
}

func (runtime *Runtime) recoverTerminalPolicyOutcome(
	ctx context.Context,
	binding Binding,
) (StepResult, bool, error) {
	recovered, err := runtime.policyRecovery.ReplayTerminalPolicyOutcome(ctx, binding)
	partial := StepResult{Binding: binding, RecoveredPolicyOutcome: recovered}
	if recovered && err == nil {
		return partial, true, fmt.Errorf(
			"%w: terminal policy outcome replay returned no registered error",
			ErrInvalidJournalState,
		)
	}
	if err != nil {
		if !recovered {
			return partial, true, errors.Join(
				ErrInvalidJournalState,
				fmt.Errorf("terminal policy outcome journal returned an error without recovery authority: %w", err),
			)
		}
		return partial, true, err
	}
	return StepResult{}, false, nil
}
