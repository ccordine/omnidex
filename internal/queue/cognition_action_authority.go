package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

type cognitionReconciliationAuthority struct {
	PolicyCallID string
	Command      cognitionruntime.ReconciliationCommand
	Receipt      cognitionruntime.ReconciliationReceipt
}

func loadExactCognitionReconciliationTx(
	ctx context.Context,
	tx pgx.Tx,
	command cognitionruntime.PrepareActionCommand,
) (cognitionReconciliationAuthority, error) {
	var authority cognitionReconciliationAuthority
	var reconciliationSHA string
	var commandJSON, receiptJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT reconciliation_sha256,policy_call_id,command_json,receipt_json
		FROM cognition_reconciliations WHERE reconciliation_id=$1
	`, command.Reconciliation.ID).Scan(
		&reconciliationSHA, &authority.PolicyCallID, &commandJSON, &receiptJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return authority, fmt.Errorf("%w: reconciliation receipt is not durable", ErrCognitionConflict)
	}
	if err != nil {
		return authority, err
	}
	if err := json.Unmarshal(commandJSON, &authority.Command); err != nil {
		return authority, fmt.Errorf("decode cognition reconciliation command: %w", err)
	}
	if err := json.Unmarshal(receiptJSON, &authority.Receipt); err != nil {
		return authority, fmt.Errorf("decode cognition reconciliation receipt: %w", err)
	}
	if reconciliationSHA != command.Reconciliation.SHA256 ||
		!reflect.DeepEqual(authority.Receipt, command.Reconciliation) ||
		authority.Receipt.ValidateFor(authority.Command) != nil ||
		command.Coordinator.Decision == nil ||
		authority.Command.SnapshotSHA256 != command.Coordinator.SnapshotSHA256 ||
		authority.Command.Projection != command.Coordinator.ContextProjection ||
		authority.Command.ActionSchema.Ref() != command.Coordinator.ActionSchema ||
		!reflect.DeepEqual(authority.Command.Decision, *command.Coordinator.Decision) {
		return authority, fmt.Errorf("%w: action preparation changed the exact reconciliation", ErrCognitionConflict)
	}
	if command.Recovery == nil {
		if authority.Command.Binding != command.Binding || authority.Command.Recovery != nil {
			return authority, fmt.Errorf("%w: action preparation changed reconciliation actor", ErrCognitionConflict)
		}
	} else if authority.PolicyCallID != command.Recovery.PolicyCallID ||
		authority.Command.Binding.Episode != command.Binding.Episode ||
		!sameQueueStepAttempt(authority.Command.Binding.Attempt, command.Binding.Attempt) ||
		authority.Command.Binding.Attempt.Attempt > command.Binding.Attempt.Attempt {
		return authority, fmt.Errorf("%w: recovered action has unrelated reconciliation authority", ErrCognitionConflict)
	}
	return authority, nil
}

func validateCognitionPreparedCommand(
	command cognitionruntime.PrepareActionCommand,
	prepared cognitionruntime.PreparedSnapshot,
	call cognitionPolicyCallRecord,
	recovery *cognitionruntime.AcceptedDecisionRecovery,
) error {
	preparedBinding := command.Binding
	if recovery != nil {
		preparedBinding.Attempt = recovery.SourceActor
	}
	if err := prepared.ValidateFor(preparedBinding); err != nil {
		return err
	}
	step := command.Coordinator
	if step.State != cognition.CoordinatorActionReady || step.Decision == nil ||
		step.Actor != command.Binding.Attempt || step.SnapshotSHA256 != prepared.Snapshot.SHA256() ||
		step.ContextProjection != prepared.Snapshot.ContextProjection() {
		return fmt.Errorf("%w: coordinator is not bound to the prepared snapshot", ErrCognitionConflict)
	}
	current := prepared.Snapshot.CurrentObligation()
	if err := step.Completion.ValidateFor(
		current, prepared.Snapshot.CurrentRevision(), current.SupportingRefs,
	); err != nil || step.Completion.Outcome != cognition.CompletionUnsatisfied {
		return fmt.Errorf("%w: coordinator completion is not an exact unresolved result", ErrCognitionConflict)
	}
	expectedBudget := prepared.Snapshot.Budget()
	if expectedBudget.RemainingPolicyCalls == 0 {
		return fmt.Errorf("%w: action cannot consume an exhausted policy budget", ErrCognitionBudgetExhausted)
	}
	expectedBudget.RemainingPolicyCalls--
	if step.RemainingBudget != expectedBudget {
		return fmt.Errorf("%w: coordinator remaining budget is not exact", ErrCognitionConflict)
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(*step.Decision)
	if err != nil {
		return err
	}
	callActorValid := call.Attempt.Actor == command.Binding.Attempt
	if recovery != nil {
		callActorValid = call.Attempt.Actor == recovery.SourceActor && command.Recovery != nil &&
			recovery.Ref() == *command.Recovery && recovery.SourcePolicyCallID == call.Attempt.ID
	}
	if call.Result == nil || call.Result.Status != cognitionpolicy.CallResultAccepted ||
		call.Result.DecisionSHA256 != decisionSHA || call.Result.ActionSchema != step.ActionSchema ||
		!callActorValid ||
		call.Attempt.SnapshotSHA256 != prepared.Snapshot.SHA256() ||
		call.Attempt.ExpectedRevision != prepared.Snapshot.CurrentRevision() ||
		call.Attempt.ObligationID != current.ID ||
		call.Attempt.RuntimeBudget != prepared.Snapshot.Budget() ||
		call.Attempt.ContextProjection != prepared.Snapshot.ContextProjection() {
		return fmt.Errorf("%w: accepted policy call does not bind action preparation", ErrCognitionConflict)
	}
	return nil
}
