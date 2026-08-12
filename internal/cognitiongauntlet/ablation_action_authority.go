package cognitiongauntlet

import "github.com/gryph/omnidex/internal/cognition"

func newAblationRegisteredAction(
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	schema cognition.ActionSchema,
	cycle uint32,
	decision cognition.CognitionDecision,
	request cognition.ActionRequest,
) (cognition.RegisteredAction, error) {
	actionDigest, err := digestJSON(struct {
		Episode  cognition.EpisodeID     `json:"episode"`
		Cycle    uint32                  `json:"cycle"`
		Request  cognition.ActionRequest `json:"request"`
		Evidence []cognition.EvidenceRef `json:"evidence"`
	}{episode.ID, cycle, request, decision.EvidenceRefs})
	if err != nil {
		return cognition.RegisteredAction{}, err
	}
	return cognition.NewRegisteredAction(
		cognition.ActionID("environment-action-"+actionDigest), actor,
		schema, request, decision.EvidenceRefs,
	)
}

func cloneAblationActionTrace(value ActionTrace) ActionTrace {
	value.Action = value.Action.Clone()
	if value.Transition != nil {
		transition := value.Transition.Clone()
		value.Transition = &transition
	}
	if value.Failure != nil {
		failure := value.Failure.Clone()
		value.Failure = &failure
	}
	return value
}

func cloneAblationActionEvidence(
	values []ablationActionEvidence,
) []ablationActionEvidence {
	result := make([]ablationActionEvidence, len(values))
	for index, value := range values {
		value.Trace = cloneAblationActionTrace(value.Trace)
		result[index] = value
	}
	return result
}
