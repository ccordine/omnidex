package queue

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

// CognitionProposalMaterializationTraceMember binds one portable payload to
// the immutable terminal-trace record that carried it.
type CognitionProposalMaterializationTraceMember struct {
	Value     CognitionProposalMaterialization               `json:"value"`
	Authority CognitionProposalMaterializationTraceAuthority `json:"trace_authority"`
}

// CognitionProposalMaterializationReconciliationAuthority is the accepted
// policy-call tuple that owns one complete proposal-materialization set.
type CognitionProposalMaterializationReconciliationAuthority struct {
	ReconciliationID string `json:"reconciliation_id"`
	PolicyCallID     string `json:"policy_call_id"`
	CallOrdinal      uint64 `json:"call_ordinal"`
}

// VerifyCognitionProposalMaterializationTraceSet proves reconciliation-wide
// totality. A successful member verification alone does not prove this set.
func VerifyCognitionProposalMaterializationTraceSet(
	members []CognitionProposalMaterializationTraceMember,
	authority CognitionProposalMaterializationReconciliationAuthority,
	snapshot cognition.RuntimeSnapshot,
	command cognitionruntime.ReconciliationCommand,
	receipt cognitionruntime.ReconciliationReceipt,
) error {
	if err := verifyCognitionProposalMaterializationSetSource(
		authority, snapshot, command, receipt,
	); err != nil {
		return err
	}
	expected := make([]int, 0, len(command.Decision.Proposals))
	for index, proposal := range command.Decision.Proposals {
		if proposal.Kind != cognition.ProposalRevision {
			expected = append(expected, index)
		}
	}
	if len(members) != len(expected) {
		return fmt.Errorf(
			"%w: proposal materialization set has %d members for %d proposals",
			ErrCognitionConflict, len(members), len(expected),
		)
	}
	if len(members) == 0 {
		return nil
	}
	first := members[0].Value
	for position, proposalIndex := range expected {
		member := members[position]
		value := member.Value
		if value.ProposalIndex != proposalIndex ||
			value.ReconciliationID != authority.ReconciliationID ||
			value.PolicyCallID != authority.PolicyCallID ||
			value.CallOrdinal != authority.CallOrdinal ||
			value.PreProposalLedgerVersion != first.PreProposalLedgerVersion ||
			value.PreProposalLedgerSHA256 != first.PreProposalLedgerSHA256 ||
			value.PreProposalLedgerJSONSHA256 != first.PreProposalLedgerJSONSHA256 ||
			!reflect.DeepEqual(value.PreProposalLedger, first.PreProposalLedger) ||
			value.OutputLedgerVersion != first.PreProposalLedgerVersion+uint64(position)+1 {
			return fmt.Errorf(
				"%w: proposal materialization set member %d changed its shared tuple",
				ErrCognitionConflict, position,
			)
		}
		if err := VerifyCognitionProposalMaterializationTrace(
			value, member.Authority, snapshot, command.Decision, command.ActionSchema,
		); err != nil {
			return fmt.Errorf(
				"%w: proposal materialization set member %d: %v",
				ErrCognitionConflict, position, err,
			)
		}
	}
	if members[len(members)-1].Value.OutputLedgerVersion != receipt.LedgerVersion {
		return fmt.Errorf(
			"%w: proposal materialization set does not reach its reconciliation receipt",
			ErrCognitionConflict,
		)
	}
	return nil
}

func verifyCognitionProposalMaterializationSetSource(
	authority CognitionProposalMaterializationReconciliationAuthority,
	snapshot cognition.RuntimeSnapshot,
	command cognitionruntime.ReconciliationCommand,
	receipt cognitionruntime.ReconciliationReceipt,
) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("%w: proposal materialization snapshot: %v", ErrCognitionConflict, err)
	}
	if err := command.Validate(); err != nil {
		return fmt.Errorf("%w: proposal materialization command: %v", ErrCognitionConflict, err)
	}
	if err := receipt.ValidateFor(command); err != nil {
		return fmt.Errorf("%w: proposal materialization receipt: %v", ErrCognitionConflict, err)
	}
	catalogSchema, exists := snapshot.ActionCatalog().Schema(command.Decision.Action.Kind)
	if authority.ReconciliationID != receipt.ID ||
		!validCognitionProposalMaterializationAuthorityID(authority.ReconciliationID, "cognition_reconciliation_") ||
		!validCognitionProposalMaterializationAuthorityID(authority.PolicyCallID, "cognition_call_") ||
		authority.CallOrdinal == 0 || authority.CallOrdinal > math.MaxInt64 ||
		command.SnapshotSHA256 != snapshot.SHA256() ||
		command.Binding.Episode.ID != snapshot.CurrentRevision().EpisodeID ||
		command.Binding.Attempt != snapshot.Attempt() ||
		command.Projection != snapshot.ContextProjection() ||
		command.Decision.ObligationID != snapshot.CurrentObligation().ID ||
		!exists || catalogSchema.Ref() != command.ActionSchema.Ref() {
		return fmt.Errorf("%w: proposal materialization reconciliation source changed", ErrCognitionConflict)
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(snapshot.EvidenceRefs()))
	for _, ref := range snapshot.EvidenceRefs() {
		available[ref] = struct{}{}
	}
	for _, ref := range command.Decision.EvidenceRefs {
		if _, exists := available[ref]; !exists {
			return fmt.Errorf("%w: proposal materialization decision evidence changed", ErrCognitionConflict)
		}
	}
	return nil
}

func validCognitionProposalMaterializationAuthorityID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) &&
		cognitionDigestPattern.MatchString(strings.TrimPrefix(value, prefix))
}
