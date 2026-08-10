package cognitionruntime

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

const ReconciliationSchemaV1 = "omnidex.cognition-runtime-reconciliation.v1"

type ReconciliationCommand struct {
	Binding        Binding                        `json:"binding"`
	SnapshotSHA256 string                         `json:"snapshot_sha256"`
	Projection     cognition.ContextProjectionRef `json:"context_projection"`
	ActionSchema   cognition.ActionSchema         `json:"action_schema"`
	Decision       cognition.CognitionDecision    `json:"decision"`
	Recovery       *AcceptedDecisionRecoveryRef   `json:"recovery,omitempty"`
}

type ReconciliationReceipt struct {
	Schema            string                       `json:"schema"`
	ID                string                       `json:"id"`
	SHA256            string                       `json:"sha256"`
	SnapshotSHA256    string                       `json:"snapshot_sha256"`
	DecisionSHA256    string                       `json:"decision_sha256"`
	ActionSchema      cognition.ActionSchemaRef    `json:"action_schema"`
	LedgerVersion     uint64                       `json:"ledger_version"`
	WorkingSetVersion uint64                       `json:"working_set_version"`
	Recovery          *AcceptedDecisionRecoveryRef `json:"recovery,omitempty"`
}

func NewReconciliationReceipt(
	command ReconciliationCommand,
	ledgerVersion uint64,
	workingSetVersion uint64,
) (ReconciliationReceipt, error) {
	if err := command.Validate(); err != nil {
		return ReconciliationReceipt{}, err
	}
	if ledgerVersion == 0 || workingSetVersion < command.Projection.WorkingSetVersion {
		return ReconciliationReceipt{}, fmt.Errorf(
			"%w: reconciliation versions do not bind persisted state", ErrInvalidJournalState,
		)
	}
	decisionSHA, err := DecisionSHA256(command.Decision)
	if err != nil {
		return ReconciliationReceipt{}, err
	}
	digest, err := valueSHA256(struct {
		Schema, SnapshotSHA256, DecisionSHA256 string
		ActionSchema                           cognition.ActionSchemaRef
		LedgerVersion, WorkingSetVersion       uint64
		Recovery                               *AcceptedDecisionRecoveryRef
	}{
		ReconciliationSchemaV1, command.SnapshotSHA256, decisionSHA, command.ActionSchema.Ref(),
		ledgerVersion, workingSetVersion, cloneRecoveryRef(command.Recovery),
	})
	if err != nil {
		return ReconciliationReceipt{}, err
	}
	return ReconciliationReceipt{
		Schema: ReconciliationSchemaV1, ID: "cognition_reconciliation_" + digest,
		SHA256: digest, SnapshotSHA256: command.SnapshotSHA256, DecisionSHA256: decisionSHA,
		ActionSchema: command.ActionSchema.Ref(), LedgerVersion: ledgerVersion,
		WorkingSetVersion: workingSetVersion, Recovery: cloneRecoveryRef(command.Recovery),
	}, nil
}

func (command ReconciliationCommand) Validate() error {
	if err := command.Binding.Validate(); err != nil {
		return err
	}
	if !validSHA256(command.SnapshotSHA256) {
		return fmt.Errorf("%w: snapshot hash is invalid", ErrInvalidJournalState)
	}
	if err := command.Projection.Validate(); err != nil {
		return fmt.Errorf("%w: projection: %v", ErrInvalidJournalState, err)
	}
	if err := command.ActionSchema.Validate(); err != nil {
		return fmt.Errorf("%w: action schema: %v", ErrInvalidJournalState, err)
	}
	if err := command.Decision.Validate(command.ActionSchema); err != nil {
		return fmt.Errorf("%w: decision: %v", ErrInvalidJournalState, err)
	}
	if command.Recovery != nil {
		if err := command.Recovery.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (command ReconciliationCommand) Clone() ReconciliationCommand {
	command.ActionSchema = command.ActionSchema.Clone()
	command.Decision = command.Decision.Clone()
	command.Recovery = cloneRecoveryRef(command.Recovery)
	return command
}

func (receipt ReconciliationReceipt) ValidateFor(command ReconciliationCommand) error {
	expected, err := NewReconciliationReceipt(command, receipt.LedgerVersion, receipt.WorkingSetVersion)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(receipt, expected) {
		return fmt.Errorf("%w: reconciliation receipt does not bind the exact decision", ErrInvalidJournalState)
	}
	return nil
}

func (receipt ReconciliationReceipt) Clone() ReconciliationReceipt {
	receipt.Recovery = cloneRecoveryRef(receipt.Recovery)
	return receipt
}

func (ref AcceptedDecisionRecoveryRef) Validate() error {
	if ref.ID != "cognition_recovery_"+ref.SHA256 || !validSHA256(ref.SHA256) ||
		!validExactPolicyCallID(ref.PolicyCallID) {
		return fmt.Errorf("%w: recovery receipt identity is invalid", ErrInvalidJournalState)
	}
	return nil
}

func cloneRecoveryRef(ref *AcceptedDecisionRecoveryRef) *AcceptedDecisionRecoveryRef {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}
