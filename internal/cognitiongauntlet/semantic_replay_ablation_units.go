package cognitiongauntlet

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func ablationSemanticUnits(root ablationEvidenceRoot) ([]ablationSemanticUnit, error) {
	units := []ablationSemanticUnit{
		ablationUnit(0, 1, 0, "ablation.root", "root", struct {
			Goal       cognition.GoalExpression      `json:"goal"`
			Completion cognition.CompletionAuthority `json:"completion"`
			Obligation cognition.Obligation          `json:"obligation"`
			Catalog    cognition.ActionCatalog       `json:"catalog"`
		}{root.Goal, root.Completion, root.Obligation, root.WorldCatalog},
			cognitionreplay.EventGoalActivated, cognitionreplay.EventObligationCreated),
		ablationUnit(0, 2, 0, "ablation.provider_bootstrap", root.BrainBootstrap.ID,
			root.BrainBootstrap, cognitionreplay.EventProviderProcessObserved),
		ablationUnit(0, 3, 0, "ablation.provider_activation", root.ProviderActivation.ID,
			root.ProviderActivation, cognitionreplay.EventProviderProcessObserved),
	}
	transitionCalls := ablationTransitionCalls(root)
	for index, transition := range root.Transitions {
		callOrdinal := transitionCalls[string(transition.ActionID)]
		phase := 50
		if index == 0 {
			callOrdinal, phase = 0, 4
		}
		events := []cognitionreplay.EventKind{cognitionreplay.EventWorldTransition}
		if index == 0 {
			events = []cognitionreplay.EventKind{cognitionreplay.EventWorldStarted}
		}
		unit := ablationUnit(callOrdinal, phase, int64(index), "ablation.transition",
			fmt.Sprintf("transition-%d-%s", index, transition.Current.SHA256), transition, events...)
		unit.revision, unit.revisionSHA = transition.Current.Number, transition.Current.SHA256
		unit.knowledge = make([]*semanticKnowledgeChange, len(events))
		units = append(units, unit)
		for observationIndex, observation := range transition.Observations {
			observationUnit := ablationUnit(
				callOrdinal, phase+1, int64(observationIndex), "ablation.transition",
				fmt.Sprintf("observation-%s", observation.ID), observation,
				cognitionreplay.EventObservationAcquired,
			)
			observationUnit.revision, observationUnit.revisionSHA = transition.Current.Number, transition.Current.SHA256
			observationUnit.knowledge = []*semanticKnowledgeChange{knowledgeChange(
				cognitionreplay.KnowledgeObservation, "observation://"+string(observation.ID),
				cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityEnvironment,
			)}
			units = append(units, observationUnit)
		}
		for effectIndex, effect := range transition.Effects {
			effectUnit := ablationUnit(
				callOrdinal, phase+2, int64(effectIndex), "ablation.transition",
				fmt.Sprintf("effect-%d-%d-%s-%s", index, effectIndex, effect.Kind, effect.ContentSHA256),
				effect, cognitionreplay.EventEvidenceAcquired,
			)
			effectUnit.revision, effectUnit.revisionSHA = transition.Current.Number, transition.Current.SHA256
			effectUnit.knowledge = []*semanticKnowledgeChange{knowledgeChange(
				cognitionreplay.KnowledgeEvidence,
				fmt.Sprintf("effect://%s/%s", effect.Kind, effect.ContentSHA256),
				cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityEnvironment,
			)}
			units = append(units, effectUnit)
		}
	}
	for index, call := range root.Calls {
		ordinal := int64(index + 1)
		units = append(units,
			ablationUnit(ordinal, 10, 0, "ablation.call_input", call.Attempt.ID+"-input", struct {
				Projection ablationProjectionEvidence `json:"projection"`
				Snapshot   semanticRuntimeSnapshot    `json:"runtime_snapshot"`
			}{call.Projection, call.Snapshot}, cognitionreplay.EventContextProjected, cognitionreplay.EventModelCalled),
			ablationUnit(ordinal, 20, 0, "ablation.call_outcome", call.Attempt.ID, struct {
				Attempt cognitionpolicy.CallAttempt `json:"attempt"`
				Result  cognitionpolicy.CallResult  `json:"result"`
			}{call.Attempt, call.Result}, ablationCallOutcomeEvents(call.Result)...),
		)
		units = append(units, ablationCallBodyUnits(ordinal, call)...)
	}
	units = append(units, ablationDispositionUnits(root)...)
	stateUnits, err := ablationStateUnits(root, transitionCalls)
	if err != nil {
		return nil, err
	}
	units = append(units, stateUnits...)
	if root.ContextBudget != nil {
		units = append(units, ablationUnit(
			int64(len(root.Calls)+1), 10, 0, "ablation.context_budget_failure",
			"context-budget", *root.ContextBudget,
			cognitionreplay.EventContextProjected, cognitionreplay.EventFailureRecorded,
		))
	}
	terminalCall := int64(len(root.Calls) + 1)
	terminalEvents := []cognitionreplay.EventKind{cognitionreplay.EventGoalFailed, cognitionreplay.EventEpisodeSealed}
	terminalKnowledge := []*semanticKnowledgeChange{
		knowledgeChange(cognitionreplay.KnowledgeGoal, "goal://active",
			cognitionreplay.KnowledgeFailed, cognitionreplay.AuthorityCode),
		nil,
	}
	if root.Terminal.GoalSatisfied {
		terminalEvents[0] = cognitionreplay.EventGoalSatisfied
		terminalKnowledge[0] = knowledgeChange(
			cognitionreplay.KnowledgeGoal, "goal://active",
			cognitionreplay.KnowledgeSatisfied, cognitionreplay.AuthorityCode,
		)
	} else if root.TerminalCause.Kind == ablationTerminalPreCallBudget ||
		root.TerminalCause.Kind == ablationTerminalCycleBudget {
		terminalEvents = append([]cognitionreplay.EventKind{cognitionreplay.EventFailureRecorded}, terminalEvents...)
		terminalKnowledge = append([]*semanticKnowledgeChange{nil}, terminalKnowledge...)
	}
	unit := ablationUnit(terminalCall, 90, 0, "ablation.terminal", "terminal", struct {
		Cause    ablationTerminalCause   `json:"cause"`
		Terminal ablationPendingTerminal `json:"terminal"`
	}{root.TerminalCause, root.Terminal}, terminalEvents...)
	unit.revision, unit.revisionSHA = root.Terminal.Revision.Number, root.Terminal.Revision.SHA256
	unit.knowledge = terminalKnowledge
	units = append(units, unit)
	sort.Slice(units, func(left, right int) bool { return ablationUnitLess(units[left], units[right]) })
	for index := range units {
		if units[index].kind == "ablation.root" {
			units[index].knowledge = []*semanticKnowledgeChange{
				knowledgeChange(
					cognitionreplay.KnowledgeGoal, "goal://active",
					cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityCode,
				),
				knowledgeChange(
					cognitionreplay.KnowledgeObligation,
					"obligation://"+string(root.Obligation.ID),
					cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityCode,
				),
			}
		}
	}
	return units, nil
}

func ablationUnit(
	call int64, phase int, sequence int64, kind, id string, value any,
	events ...cognitionreplay.EventKind,
) ablationSemanticUnit {
	return ablationSemanticUnit{
		callOrdinal: call, phase: phase, sequence: sequence,
		kind: kind, id: id, value: value, events: events,
	}
}

func ablationCallOutcomeEvents(result cognitionpolicy.CallResult) []cognitionreplay.EventKind {
	values := []cognitionreplay.EventKind{
		cognitionreplay.EventProviderRequestDisposition,
		cognitionreplay.EventModelCallDisposition,
	}
	switch result.Status {
	case cognitionpolicy.CallResultAccepted:
		return append(values, cognitionreplay.EventDecisionAccepted)
	case cognitionpolicy.CallResultRejected:
		return append(values, cognitionreplay.EventDecisionRejected)
	default:
		return append(values, cognitionreplay.EventFailureRecorded)
	}
}

func ablationCallBodyUnits(callOrdinal int64, call ablationCallEvidence) []ablationSemanticUnit {
	values := []ablationSemanticUnit{}
	if call.Evidence.ModelResponse != nil {
		values = append(values, ablationUnit(callOrdinal, 21, 0, "ablation.model_response",
			call.Attempt.ID+"-model-response", *call.Evidence.ModelResponse,
			cognitionreplay.EventEvidenceAcquired))
	}
	if call.Evidence.ProviderIdentity != nil {
		values = append(values, ablationUnit(callOrdinal, 22, 0, "ablation.provider_identity",
			call.Attempt.ID+"-provider-identity", *call.Evidence.ProviderIdentity,
			cognitionreplay.EventProviderRequestDisposition, cognitionreplay.EventEvidenceAcquired))
	}
	if call.Evidence.ProviderResponseCapture != nil {
		values = append(values, ablationUnit(callOrdinal, 23, 0, "ablation.provider_response_capture",
			call.Attempt.ID+"-provider-capture", *call.Evidence.ProviderResponseCapture,
			cognitionreplay.EventEvidenceAcquired))
	}
	if call.Evidence.ProviderGeneration != nil {
		values = append(values, ablationUnit(callOrdinal, 24, 0, "ablation.provider_generation",
			call.Attempt.ID+"-provider-generation", *call.Evidence.ProviderGeneration,
			cognitionreplay.EventEvidenceAcquired))
	}
	return values
}

func ablationUnitLess(left, right ablationSemanticUnit) bool {
	if left.callOrdinal != right.callOrdinal {
		return left.callOrdinal < right.callOrdinal
	}
	if left.phase != right.phase {
		return left.phase < right.phase
	}
	if left.sequence != right.sequence {
		return left.sequence < right.sequence
	}
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	return left.id < right.id
}
