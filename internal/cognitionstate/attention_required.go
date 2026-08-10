package cognitionstate

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/workingset"
)

func applyRequiredAttention(
	input ReconciliationInput,
	candidates []attentionCandidate,
	evidence map[cognition.EvidenceRef]EvidenceMaterial,
) ([]attentionCandidate, error) {
	if len(input.RequiredAttention) > MaxAdvisoryRetains {
		return nil, fmt.Errorf("%w: required retained evidence exceeds %d", ErrReconciliationCapacity, MaxAdvisoryRetains)
	}
	seen := make(map[cognition.EvidenceRef]struct{}, len(input.RequiredAttention))
	available := make(map[cognition.EvidenceRef]struct{}, len(input.State.EvidenceRefs()))
	for _, ref := range input.State.EvidenceRefs() {
		available[ref] = struct{}{}
	}
	for index, request := range input.RequiredAttention {
		if request.Operation != cognition.AttentionRetain ||
			(request.Scope != cognition.AttentionScopeObligation && request.Scope != cognition.AttentionScopeEpisode) {
			return nil, fmt.Errorf("%w: required attention %d is not an admitted durable retention", ErrInvalidReconciliation, index)
		}
		if err := validateAdvisoryRequest(request, available); err != nil {
			return nil, fmt.Errorf("%w: required attention %d: %v", ErrInvalidReconciliation, index, err)
		}
		if _, duplicate := seen[request.TargetRef]; duplicate {
			return nil, fmt.Errorf("%w: required attention %d duplicates evidence", ErrInvalidReconciliation, index)
		}
		seen[request.TargetRef] = struct{}{}
		material, exists := evidence[request.TargetRef]
		if !exists {
			return nil, fmt.Errorf("%w: required retained evidence %q is unavailable", ErrMissingMaterial, request.TargetRef.ObservationID)
		}
		membership, err := AttentionMembership(
			request.Scope, input.WorkingSet.Scope, input.State.Obligation().ID, input.State.SHA256(),
		)
		if err != nil {
			return nil, err
		}
		candidate := evidenceCandidate(material, true)
		candidate.pinned = false
		candidate.memberships = []workingset.Membership{membership}
		candidates, err = mergeAttentionCandidate(candidates, candidate)
		if err != nil {
			return nil, err
		}
	}
	return candidates, nil
}
