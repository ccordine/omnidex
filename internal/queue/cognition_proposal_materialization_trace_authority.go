package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

const CognitionProposalMaterializationTracePhase = 42

// CognitionProposalMaterializationTraceAuthority is the exact portable
// reconciliation and sealed-record tuple that owns one materialization.
type CognitionProposalMaterializationTraceAuthority struct {
	ReconciliationID string `json:"reconciliation_id"`
	PolicyCallID     string `json:"policy_call_id"`
	CallOrdinal      uint64 `json:"call_ordinal"`
	Phase            int    `json:"phase"`
	Sequence         int64  `json:"sequence"`
	ID               string `json:"id"`
	SHA256           string `json:"sha256"`
}

func (authority CognitionProposalMaterializationTraceAuthority) validateFor(
	value CognitionProposalMaterialization,
) error {
	if authority.ReconciliationID != value.ReconciliationID ||
		authority.PolicyCallID != value.PolicyCallID ||
		authority.CallOrdinal != value.CallOrdinal ||
		authority.Phase != CognitionProposalMaterializationTracePhase ||
		authority.Sequence != int64(value.ProposalIndex) || authority.ID != value.ID {
		return fmt.Errorf("%w: proposal materialization trace tuple changed", ErrCognitionConflict)
	}
	raw, err := exactjson.Canonical(value)
	if err != nil || authority.SHA256 != cognitionPayloadSHA(raw) {
		return fmt.Errorf(
			"%w: proposal materialization trace payload identity changed: %v",
			ErrCognitionConflict, err,
		)
	}
	return nil
}
