package cognitiongauntlet

import (
	"sort"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func knowledgeDisposition(kind cognitionreplay.EventKind) (
	cognitionreplay.KnowledgeKind,
	cognitionreplay.KnowledgeStatus,
	cognitionreplay.KnowledgeAuthority,
	bool,
) {
	switch kind {
	case cognitionreplay.EventObservationAcquired:
		return cognitionreplay.KnowledgeObservation, cognitionreplay.KnowledgeActive,
			cognitionreplay.AuthorityEnvironment, true
	case cognitionreplay.EventEvidenceAcquired:
		return cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgeActive,
			cognitionreplay.AuthorityTool, true
	case cognitionreplay.EventHypothesisCreated:
		return cognitionreplay.KnowledgeBelief, cognitionreplay.KnowledgeActive,
			cognitionreplay.AuthorityModelProposal, true
	case cognitionreplay.EventHypothesisRejected:
		return cognitionreplay.KnowledgeBelief, cognitionreplay.KnowledgeRejected,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventGoalActivated:
		return cognitionreplay.KnowledgeGoal, cognitionreplay.KnowledgeActive,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventGoalSatisfied:
		return cognitionreplay.KnowledgeGoal, cognitionreplay.KnowledgeSatisfied,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventGoalFailed:
		return cognitionreplay.KnowledgeGoal, cognitionreplay.KnowledgeFailed,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventObligationCreated, cognitionreplay.EventObligationChanged:
		return cognitionreplay.KnowledgeObligation, cognitionreplay.KnowledgeActive,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventWorkingSetAttached, cognitionreplay.EventWorkingSetReacquired,
		cognitionreplay.EventWorkingSetRetained, cognitionreplay.EventWorkingSetTouched:
		return cognitionreplay.KnowledgeWorkingSet, cognitionreplay.KnowledgeActive,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventWorkingSetReleased:
		return cognitionreplay.KnowledgeWorkingSet, cognitionreplay.KnowledgeReleased,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventWorkingSetInvalidated:
		return cognitionreplay.KnowledgeWorkingSet, cognitionreplay.KnowledgeStale,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventContextProjected:
		return cognitionreplay.KnowledgeProjection, cognitionreplay.KnowledgeActive,
			cognitionreplay.AuthorityCode, true
	case cognitionreplay.EventFailureRecorded, cognitionreplay.EventEpisodeCanceled:
		return cognitionreplay.KnowledgeFailure, cognitionreplay.KnowledgeFailed,
			cognitionreplay.AuthorityCode, true
	default:
		return "", "", "", false
	}
}

func semanticReplaySortedEntries(
	values map[string]cognitionreplay.KnowledgeEntry,
) []cognitionreplay.KnowledgeEntry {
	result := make([]cognitionreplay.KnowledgeEntry, 0, len(values))
	for _, entry := range values {
		result = append(result, entry)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Kind != result[right].Kind {
			return result[left].Kind < result[right].Kind
		}
		return result[left].Ref < result[right].Ref
	})
	return result
}
