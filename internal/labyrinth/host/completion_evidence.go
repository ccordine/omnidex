package host

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

// completionEvidence binds satisfaction to observations emitted by the exact
// committed revision being evaluated. It selects only refs already present in
// the code-owned snapshot packet and never inspects private oracle labels.
func completionEvidence(
	request cognitionruntime.CompletionRequest,
	stored storedEpisode,
	history []storedAction,
	satisfied bool,
) ([]cognition.EvidenceRef, error) {
	if !satisfied {
		return []cognition.EvidenceRef{}, nil
	}
	transition, err := completionTransition(stored, history, request.Revision)
	if err != nil {
		return nil, err
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(request.EvidenceRefs))
	for _, ref := range request.EvidenceRefs {
		available[ref] = struct{}{}
	}
	evidence := make([]cognition.EvidenceRef, 0, len(transition.Observations))
	for _, observation := range transition.Observations {
		if observation.Revision != request.Revision ||
			(transition.ActionID != "" && observation.ActionID != transition.ActionID) {
			return nil, fmt.Errorf("%w: completion transition observation authority changed", ErrReceiptCorrupt)
		}
		ref := observation.EvidenceRef()
		if _, exists := available[ref]; exists {
			evidence = append(evidence, ref)
		}
	}
	if len(evidence) == 0 {
		return nil, fmt.Errorf(
			"%w: satisfied completion lacks its exact transition observation in the snapshot packet",
			cognition.ErrInvalidEvidence,
		)
	}
	return evidence, nil
}

func completionTransition(
	stored storedEpisode,
	history []storedAction,
	revision cognition.WorldRevision,
) (cognition.Transition, error) {
	if revision == stored.Start.Current {
		return stored.Start.Clone(), nil
	}
	if len(history) == 0 {
		return cognition.Transition{}, fmt.Errorf("%w: completion transition is absent", ErrReceiptCorrupt)
	}
	latest := history[len(history)-1].Receipt.Transition
	if latest == nil || latest.Current != revision {
		return cognition.Transition{}, fmt.Errorf(
			"%w: latest committed transition cannot evidence current satisfaction", ErrReceiptCorrupt,
		)
	}
	return latest.Clone(), nil
}
