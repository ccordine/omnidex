package cognitionruntime

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

func (runtime *Runtime) recoverAcceptedDecision(
	ctx context.Context,
	binding Binding,
	recovery AcceptedDecisionRecovery,
) (StepResult, error) {
	partial := StepResult{
		Binding: binding, Revision: recovery.Prepared.Snapshot.CurrentRevision(),
		RecoveredDecision: true,
	}
	if err := recovery.ValidateFor(binding); err != nil {
		return partial, err
	}
	prepared := recovery.Prepared.clone()
	if prepared.EnvironmentTerminal {
		return partial, fmt.Errorf("%w: terminal environment cannot execute a recovered decision", ErrInvalidJournalState)
	}
	completion, err := runtime.completion.Evaluate(ctx, completionRequest(prepared, binding))
	if err != nil {
		return partial, fmt.Errorf("evaluate recovered cognition completion: %w", err)
	}
	if err := validateCompletionResult(prepared, completion); err != nil {
		return partial, err
	}
	if completion.Outcome != cognition.CompletionUnsatisfied {
		return partial, fmt.Errorf("%w: recovered decision is no longer unresolved", ErrInvalidJournalState)
	}
	step, err := recoveredCoordinatorStep(binding, recovery, completion)
	if err != nil {
		return partial, err
	}
	reconcile := ReconciliationCommand{
		Binding: binding, SnapshotSHA256: prepared.Snapshot.SHA256(),
		Projection: prepared.Snapshot.ContextProjection(), ActionSchema: recovery.ActionSchema.Clone(),
		Decision: recovery.Decision.Clone(), Recovery: recoveryRefPointer(recovery.Ref()),
	}
	var receipt ReconciliationReceipt
	if recovery.ExistingReconciliation != nil {
		receipt = recovery.ExistingReconciliation.Receipt.Clone()
	} else {
		receipt, err = runtime.reconciler.Reconcile(ctx, reconcile.Clone())
		if err != nil {
			return partial, fmt.Errorf("reconcile recovered cognition decision: %w", err)
		}
		if err := receipt.ValidateFor(reconcile); err != nil {
			return partial, err
		}
	}
	record, err := runtime.actions.PrepareAction(ctx, PrepareActionCommand{
		Binding: binding, Coordinator: cloneCoordinatorStep(step), Reconciliation: receipt,
		Recovery: recoveryRefPointer(recovery.Ref()),
	})
	if err != nil {
		return partial, fmt.Errorf("prepare recovered cognition action: %w", err)
	}
	partial.ActionID = record.Action.ID
	if err := validatePreparedAction(prepared, binding, step, record); err != nil {
		return partial, err
	}
	return runtime.execute(ctx, binding, record, true, false, true, false)
}

func recoveredCoordinatorStep(
	binding Binding,
	recovery AcceptedDecisionRecovery,
	completion cognition.CompletionResult,
) (cognition.CoordinatorStep, error) {
	budget := recovery.Prepared.Snapshot.Budget()
	if budget.RemainingPolicyCalls == 0 {
		return cognition.CoordinatorStep{}, fmt.Errorf(
			"%w: recovered policy decision has an exhausted source budget", ErrInvalidJournalState,
		)
	}
	budget.RemainingPolicyCalls--
	decision := recovery.Decision.Clone()
	step := cognition.CoordinatorStep{
		State: cognition.CoordinatorActionReady, SnapshotSHA256: recovery.Prepared.Snapshot.SHA256(),
		Decision: &decision, ActionSchema: recovery.ActionSchema.Ref(), Actor: binding.Attempt,
		ContextProjection: recovery.Prepared.Snapshot.ContextProjection(),
		Completion:        completion.Clone(), RemainingBudget: budget,
	}
	if step.Decision.ObligationID != recovery.Prepared.Snapshot.CurrentObligation().ID ||
		!reflect.DeepEqual(*step.Decision, recovery.Decision) {
		return cognition.CoordinatorStep{}, fmt.Errorf(
			"%w: recovered coordinator step changed the accepted decision", ErrInvalidJournalState,
		)
	}
	return step, nil
}

func recoveryRefPointer(ref AcceptedDecisionRecoveryRef) *AcceptedDecisionRecoveryRef {
	copy := ref
	return &copy
}
