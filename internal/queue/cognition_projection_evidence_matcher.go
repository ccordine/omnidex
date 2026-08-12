package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

// ResolveCognitionProjectionEvidenceRef matches a selected Context Projection
// reference with the exact task reference derivation used by policy snapshot
// preparation. It rejects absent and ambiguous matches.
func ResolveCognitionProjectionEvidenceRef(
	selected taskstate.Ref,
	candidates []cognition.EvidenceRef,
) (cognition.EvidenceRef, error) {
	if taskstate.ValidateRef(selected) != nil || selected.Relation != taskstate.RefEvidence {
		return cognition.EvidenceRef{}, fmt.Errorf(
			"%w: projected cognition evidence task reference is invalid",
			ErrCognitionConflict,
		)
	}
	var matched cognition.EvidenceRef
	matches := 0
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return cognition.EvidenceRef{}, fmt.Errorf(
				"%w: projected cognition evidence candidate is invalid",
				ErrCognitionConflict,
			)
		}
		if cognitionEvidenceTaskRefs([]cognition.EvidenceRef{candidate})[0] == selected {
			matched = candidate
			matches++
		}
	}
	if matches != 1 {
		return cognition.EvidenceRef{}, fmt.Errorf(
			"%w: projected cognition evidence task reference resolved %d candidates",
			ErrCognitionConflict, matches,
		)
	}
	return matched, nil
}
