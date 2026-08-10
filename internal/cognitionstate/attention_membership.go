package cognitionstate

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/workingset"
)

const attentionObligationScopePrefix = "obligation-"
const attentionDecisionScopePrefix = "cognition-decision-"

func AttentionMembership(
	scope cognition.AttentionScope,
	root workingset.Scope,
	obligationID cognition.ObligationID,
	decisionSHA256 string,
) (workingset.Membership, error) {
	if root.Kind != workingset.ScopeJob || !validAttentionScopeID(string(root.ID)) {
		return workingset.Membership{}, fmt.Errorf("%w: working-set root scope is invalid", ErrInvalidReconciliation)
	}
	switch scope {
	case cognition.AttentionScopeDecision:
		if !validMappingDigest(decisionSHA256) {
			return workingset.Membership{}, fmt.Errorf("%w: decision scope hash is invalid", ErrInvalidReconciliation)
		}
		return workingset.Membership{
			Scope:     workingset.Scope{Kind: workingset.ScopeCall, ID: workingset.ScopeID(attentionDecisionScopePrefix + decisionSHA256)},
			Retention: workingset.RetentionCall,
		}, nil
	case cognition.AttentionScopeObligation:
		if !validAttentionScopeID(string(obligationID)) {
			return workingset.Membership{}, fmt.Errorf("%w: obligation scope is invalid", ErrInvalidReconciliation)
		}
		return workingset.Membership{
			Scope:     workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(attentionObligationScopePrefix + string(obligationID))},
			Retention: workingset.RetentionTask,
		}, nil
	case cognition.AttentionScopeEpisode:
		return workingset.Membership{Scope: root, Retention: workingset.RetentionJob}, nil
	default:
		return workingset.Membership{}, fmt.Errorf("%w: attention scope %q is not registered", ErrInvalidReconciliation, scope)
	}
}

func AttentionMembershipApplies(
	membership workingset.Membership,
	root workingset.Scope,
	current cognition.ObligationID,
) bool {
	episode, err := AttentionMembership(cognition.AttentionScopeEpisode, root, current, mappingZeroDigest)
	if err == nil && membership == episode {
		return true
	}
	obligation, err := AttentionMembership(cognition.AttentionScopeObligation, root, current, mappingZeroDigest)
	return err == nil && membership == obligation
}

func validAttentionScopeID(value string) bool {
	return value != "" && len(value) <= 256 && value == strings.TrimSpace(value) &&
		utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

const mappingZeroDigest = "0000000000000000000000000000000000000000000000000000000000000000"
