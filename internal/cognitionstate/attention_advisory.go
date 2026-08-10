package cognitionstate

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func applyAdvisoryRequests(
	input ReconciliationInput,
	mandatory []attentionCandidate,
	evidence map[cognition.EvidenceRef]EvidenceMaterial,
) ([]attentionCandidate, []AdvisoryOutcome, error) {
	if len(input.Attention) > cognition.MaxAttentionRequests {
		return nil, nil, fmt.Errorf("%w: attention request count exceeds %d", ErrInvalidReconciliation, cognition.MaxAttentionRequests)
	}
	protected := make(map[string]struct{}, len(mandatory))
	protectedIndex := make(map[string]int, len(mandatory))
	for index, candidate := range mandatory {
		protected[taskstate.RefIdentity(candidate.ref)] = struct{}{}
		protectedIndex[taskstate.RefIdentity(candidate.ref)] = index
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(input.State.EvidenceRefs()))
	for _, ref := range input.State.EvidenceRefs() {
		available[ref] = struct{}{}
	}
	resident := make(map[string]struct{})
	for _, item := range input.WorkingSet.Items {
		if item.State == workingset.ItemResident {
			resident[taskstate.RefIdentity(item.Ref)] = struct{}{}
		}
	}
	optional := make([]attentionCandidate, 0)
	outcomes := make([]AdvisoryOutcome, 0, len(input.Attention))
	seen := make(map[cognition.EvidenceRef]cognition.AttentionOperation)
	forcedCapacity, err := validateCapacityRejections(input)
	if err != nil {
		return nil, nil, err
	}
	materialBytes := candidateBytes(mandatory)
	retainsAccepted := len(input.RequiredAttention)
	for index, request := range input.Attention {
		if err := validateAdvisoryRequest(request, available); err != nil {
			return nil, nil, fmt.Errorf("%w: request %d: %v", ErrInvalidReconciliation, index, err)
		}
		if previous, duplicate := seen[request.TargetRef]; duplicate {
			return nil, nil, fmt.Errorf("%w: evidence has both %s and %s requests", ErrInvalidReconciliation, previous, request.Operation)
		}
		seen[request.TargetRef] = request.Operation
		identity := taskstate.RefIdentity(evidenceLedgerRef(request.TargetRef))
		outcome := AdvisoryOutcome{Request: request}
		if _, rejected := forcedCapacity[request.TargetRef]; rejected {
			outcome.Disposition = AdvisoryRejectedCapacity
			outcome.Reason = "The exact policy envelope cannot admit this scoped retention."
			outcomes = append(outcomes, outcome)
			continue
		}
		switch request.Operation {
		case cognition.AttentionRelease:
			if _, isProtected := protected[identity]; isProtected {
				outcome.Disposition = AdvisoryRejectedProtected
				outcome.Reason = "Code-owned causal retention protects this evidence."
			} else if _, exists := resident[identity]; !exists {
				outcome.Disposition = AdvisoryRejectedUnavailable
				outcome.Reason = "The requested evidence is not resident."
			} else {
				outcome.Disposition = AdvisoryAccepted
				outcome.Reason = "Code accepted release of non-causal evidence."
			}
		case cognition.AttentionRetain:
			if retainsAccepted >= MaxAdvisoryRetains {
				outcome.Disposition = AdvisoryRejectedCapacity
				outcome.Reason = "The scoped retention cap is exhausted."
				break
			}
			if _, isProtected := protected[identity]; isProtected {
				membership, membershipErr := AttentionMembership(
					request.Scope, input.WorkingSet.Scope, input.State.Obligation().ID, input.State.SHA256(),
				)
				if membershipErr != nil {
					return nil, nil, membershipErr
				}
				candidate := &mandatory[protectedIndex[identity]]
				if !candidateHasMembership(*candidate, membership) {
					candidate.memberships = append(candidate.memberships, membership)
				}
				outcome.Disposition = AdvisoryAccepted
				outcome.Reason = "Evidence is already protected by code-owned causal retention."
				retainsAccepted++
				break
			}
			material, exists := evidence[request.TargetRef]
			if !exists {
				outcome.Disposition = AdvisoryRejectedUnavailable
				outcome.Reason = "Exact evidence material is unavailable."
				break
			}
			if len(optional) >= MaxAdvisoryRetains || len(mandatory)+len(optional)+1 > MaxContextItems ||
				materialBytes+len(material.Content) > MaxContextMaterialBytes {
				outcome.Disposition = AdvisoryRejectedCapacity
				outcome.Reason = "The advisory retention cap is exhausted."
				break
			}
			candidate := evidenceCandidate(material, request.Scope != cognition.AttentionScopeDecision)
			candidate.pinned = false
			candidate.advisory = true
			membership, err := AttentionMembership(
				request.Scope, input.WorkingSet.Scope, input.State.Obligation().ID, input.State.SHA256(),
			)
			if err != nil {
				return nil, nil, err
			}
			candidate.memberships = []workingset.Membership{membership}
			optional = append(optional, candidate)
			materialBytes += len(candidate.content)
			outcome.Disposition = AdvisoryAccepted
			outcome.Reason = "Code accepted bounded retention of non-causal evidence."
			retainsAccepted++
		}
		outcomes = append(outcomes, outcome)
	}
	return optional, outcomes, nil
}

func validateCapacityRejections(input ReconciliationInput) (map[cognition.EvidenceRef]struct{}, error) {
	requested := make(map[cognition.EvidenceRef]cognition.AttentionRequest, len(input.Attention))
	for _, request := range input.Attention {
		requested[request.TargetRef] = request
	}
	result := make(map[cognition.EvidenceRef]struct{}, len(input.CapacityRejected))
	for index, ref := range input.CapacityRejected {
		request, exists := requested[ref]
		if !exists || request.Operation != cognition.AttentionRetain || request.Scope == cognition.AttentionScopeDecision {
			return nil, fmt.Errorf("%w: capacity rejection %d has no durable retain request", ErrInvalidReconciliation, index)
		}
		if _, duplicate := result[ref]; duplicate {
			return nil, fmt.Errorf("%w: capacity rejection %d is duplicated", ErrInvalidReconciliation, index)
		}
		result[ref] = struct{}{}
	}
	return result, nil
}

func validateAdvisoryRequest(
	request cognition.AttentionRequest,
	available map[cognition.EvidenceRef]struct{},
) error {
	switch request.Operation {
	case cognition.AttentionRetain, cognition.AttentionRelease:
	default:
		return fmt.Errorf("operation %q is not registered", request.Operation)
	}
	switch request.Scope {
	case cognition.AttentionScopeDecision, cognition.AttentionScopeObligation, cognition.AttentionScopeEpisode:
	default:
		return fmt.Errorf("scope %q is not registered", request.Scope)
	}
	if err := request.TargetRef.Validate(); err != nil {
		return err
	}
	if _, exists := available[request.TargetRef]; !exists {
		return fmt.Errorf("target evidence is outside the runtime snapshot")
	}
	if request.Reason == "" || len(request.Reason) > cognition.MaxAttentionReasonBytes ||
		!utf8.ValidString(request.Reason) || strings.ContainsRune(request.Reason, 0) ||
		strings.TrimSpace(request.Reason) != request.Reason {
		return fmt.Errorf("reason is not exact bounded text")
	}
	return nil
}

func candidateBytes(candidates []attentionCandidate) int {
	total := 0
	for _, candidate := range candidates {
		total += len(candidate.content)
	}
	return total
}
