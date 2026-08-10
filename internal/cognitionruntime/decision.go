package cognitionruntime

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

func (runtime *Runtime) decideAndExecute(
	ctx context.Context,
	binding Binding,
	prepared PreparedSnapshot,
	completion cognition.CompletionResult,
) (StepResult, error) {
	partial := StepResult{
		Binding: binding, Revision: prepared.Snapshot.CurrentRevision(),
	}
	step, err := runtime.coordinator.Step(
		ctx, prepared.Snapshot, completion, prepared.CompletionEvidenceRefs,
	)
	if err != nil {
		partial.PolicyCalled = step.PolicyCalled
		return partial, err
	}
	partial.PolicyCalled = step.PolicyCalled
	if err := validateCoordinatorAction(prepared, binding, completion, step); err != nil {
		return partial, err
	}
	decision := step.Decision.Clone()
	schema, exists := prepared.Snapshot.ActionCatalog().Schema(decision.Action.Kind)
	if !exists {
		return partial, fmt.Errorf("%w: action schema disappeared after coordinator validation", ErrInvalidJournalState)
	}
	reconcile := ReconciliationCommand{
		Binding: binding, SnapshotSHA256: prepared.Snapshot.SHA256(),
		Projection: prepared.Snapshot.ContextProjection(), ActionSchema: schema, Decision: decision,
	}
	if err := reconcile.Validate(); err != nil {
		return partial, err
	}
	receipt, err := runtime.reconciler.Reconcile(ctx, reconcile.Clone())
	if err != nil {
		return partial, fmt.Errorf("reconcile cognition decision: %w", err)
	}
	if err := receipt.ValidateFor(reconcile); err != nil {
		return partial, err
	}
	record, err := runtime.actions.PrepareAction(ctx, PrepareActionCommand{
		Binding: binding, Coordinator: cloneCoordinatorStep(step), Reconciliation: receipt,
	})
	if err != nil {
		return partial, fmt.Errorf("prepare cognition action: %w", err)
	}
	partial.ActionID = record.Action.ID
	if err := validatePreparedAction(prepared, binding, step, record); err != nil {
		return partial, err
	}
	return runtime.execute(ctx, binding, record, true, step.PolicyCalled, false, false)
}

func validateCoordinatorSatisfied(
	prepared PreparedSnapshot,
	binding Binding,
	completion cognition.CompletionResult,
	step cognition.CoordinatorStep,
) error {
	if step.State != cognition.CoordinatorObligationSatisfied || step.Decision != nil ||
		step.SnapshotSHA256 != prepared.Snapshot.SHA256() || step.Actor != binding.Attempt ||
		step.ContextProjection != prepared.Snapshot.ContextProjection() ||
		!reflect.DeepEqual(step.Completion, completion) {
		return fmt.Errorf("%w: satisfied coordinator result is not bound to prepared state", ErrInvalidProgress)
	}
	return nil
}

func cloneCoordinatorStep(step cognition.CoordinatorStep) cognition.CoordinatorStep {
	step.Completion = step.Completion.Clone()
	if step.Decision != nil {
		decision := step.Decision.Clone()
		step.Decision = &decision
	}
	return step
}

func validateCoordinatorAction(
	prepared PreparedSnapshot,
	binding Binding,
	completion cognition.CompletionResult,
	step cognition.CoordinatorStep,
) error {
	if step.State != cognition.CoordinatorActionReady || step.Decision == nil ||
		step.SnapshotSHA256 != prepared.Snapshot.SHA256() || step.Actor != binding.Attempt ||
		step.ContextProjection != prepared.Snapshot.ContextProjection() ||
		!reflect.DeepEqual(step.Completion, completion) {
		return fmt.Errorf("%w: coordinator action is not bound to the prepared state", ErrInvalidJournalState)
	}
	schema, exists := prepared.Snapshot.ActionCatalog().Schema(step.Decision.Action.Kind)
	if !exists || step.ActionSchema != schema.Ref() {
		return fmt.Errorf("%w: coordinator selected an unbound action schema", ErrInvalidJournalState)
	}
	return nil
}

func validatePreparedAction(
	prepared PreparedSnapshot,
	binding Binding,
	step cognition.CoordinatorStep,
	record ActionRecord,
) error {
	if err := record.ValidateFor(binding); err != nil {
		return err
	}
	if record.Status != ActionPrepared || record.Action.Actor != binding.Attempt ||
		record.ExpectedRevision != prepared.Snapshot.CurrentRevision() ||
		record.SnapshotSHA256 != prepared.Snapshot.SHA256() ||
		record.ContextProjection != prepared.Snapshot.ContextProjection() ||
		record.Schema.Ref() != step.ActionSchema || !reflect.DeepEqual(record.Decision, *step.Decision) {
		return fmt.Errorf("%w: prepared action differs from the exact coordinator decision", ErrInvalidJournalState)
	}
	return nil
}
