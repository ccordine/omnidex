package repositoryobjective

import (
	"fmt"
	"reflect"
)

type objectiveState struct {
	subjectResolved      bool
	declarationObserved  bool
	directRelationsKnown bool
	applicableTestsKnown bool
}

func (state objectiveState) satisfies(predicate AcceptancePredicate) bool {
	switch predicate {
	case AcceptanceSubjectResolved:
		return state.subjectResolved
	case AcceptanceDeclarationObserved:
		return state.declarationObserved
	case AcceptanceDirectRelationsKnown:
		return state.directRelationsKnown
	case AcceptanceApplicableTestsKnown:
		return state.applicableTestsKnown
	default:
		return false
	}
}

func evaluateCompletion(
	objective Objective,
	state objectiveState,
	subject SubjectFact,
	selected subjectCandidate,
	analysisID string,
) ([]AcceptancePredicate, error) {
	if subject.ObjectiveID != objective.ID || subject.AnalysisID != analysisID ||
		!reflect.DeepEqual(subject.Acceptance, objective.Acceptance) ||
		!reflect.DeepEqual(subject.Symbol, selected.evidence) {
		return nil, fmt.Errorf("%w: subject fact is not bound to the exact objective authority", ErrObjectiveIncomplete)
	}
	switch subject.Authority {
	case SubjectAuthorityDeterministic:
		if subject.GapID != "" || subject.CandidateID != "" {
			return nil, fmt.Errorf("%w: deterministic subject carries semantic authority", ErrObjectiveIncomplete)
		}
	case SubjectAuthoritySemantic:
		if subject.GapID == "" || subject.CandidateID == "" {
			return nil, fmt.Errorf("%w: semantic subject lacks gap authority", ErrObjectiveIncomplete)
		}
	default:
		return nil, fmt.Errorf("%w: subject has invalid authority %q", ErrObjectiveIncomplete, subject.Authority)
	}
	satisfied := make([]AcceptancePredicate, 0, len(objective.Acceptance))
	for _, predicate := range objective.Acceptance {
		if !state.satisfies(predicate) {
			return nil, fmt.Errorf("%w: acceptance predicate %q is unsatisfied", ErrObjectiveIncomplete, predicate)
		}
		satisfied = append(satisfied, predicate)
	}
	return satisfied, nil
}
