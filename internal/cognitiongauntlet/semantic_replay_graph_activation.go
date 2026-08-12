package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func (state *semanticReplayState) activateObligationGraph(
	version uint64,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	graph, exists := state.graphs[version]
	if !exists || version != state.activeGraphVersion+1 {
		return nil, fmt.Errorf("semantic obligation graph activation is out of sequence")
	}
	if version == 1 && !semanticInitialGraphMatchesGoal(graph, state.goal) {
		return nil, fmt.Errorf("semantic initial obligation graph changed the public goal")
	}
	state.activeGraphVersion = version
	drafts := make([]semanticEventDraft, 0, len(graph.Obligations)+1)
	if len(state.obligations) == 0 {
		drafts = append(drafts, semanticEventDraft{
			Kind: cognitionreplay.EventGoalActivated, Payload: source.Payload,
			Knowledge: knowledgeChange(
				cognitionreplay.KnowledgeGoal,
				"goal://"+string(state.trace.Header.EpisodeID),
				cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityCode,
			),
		})
	}
	for _, obligation := range graph.Obligations {
		prior, exists := state.obligations[obligation.ID]
		if exists && reflect.DeepEqual(prior, obligation) {
			continue
		}
		kind := cognitionreplay.EventObligationChanged
		if !exists {
			kind = cognitionreplay.EventObligationCreated
		}
		draft, err := state.typedDraft(
			kind, source, obligation, nil,
			knowledgeChange(
				cognitionreplay.KnowledgeObligation, "obligation://"+string(obligation.ID),
				semanticObligationKnowledgeStatus(obligation.Status), cognitionreplay.AuthorityCode,
			),
		)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, draft)
		state.obligations[obligation.ID] = obligation.Clone()
	}
	if len(drafts) == 0 {
		drafts = append(drafts, sourceDraft(cognitionreplay.EventObligationChanged, source))
	}
	return drafts, nil
}

func semanticInitialGraphMatchesGoal(
	graph cognition.ObligationGraphSnapshot,
	goal cognition.GoalExpression,
) bool {
	for _, obligation := range graph.Obligations {
		if obligation.ID == graph.RootID {
			return obligation.ParentID == "" && reflect.DeepEqual(obligation.Desired, goal)
		}
	}
	return false
}

func (state *semanticReplayState) activateReconciliationGraph(
	reconciliationID string,
	terminal bool,
) ([]semanticEventDraft, error) {
	mutation, exists := state.graphMutations[reconciliationID]
	if !exists {
		return nil, nil
	}
	if terminal || mutation.version != state.activeGraphVersion+1 {
		return nil, fmt.Errorf("semantic graph mutation is not caused by one nonterminal action")
	}
	drafts := make([]semanticEventDraft, 0, 2)
	if mutation.kind == "plan_revision" {
		planSource, exists := state.deferredSources[mutation.applicationSource]
		if !exists || planSource.Kind != "plan_revision" {
			return nil, fmt.Errorf("semantic plan revision lacks its deferred application source")
		}
		drafts = append(drafts,
			sourceDraft(cognitionreplay.EventPlanRevised, planSource).withSource(planSource),
		)
	}
	source, deferred := state.deferredSourceForGraph(mutation.version)
	if !deferred {
		return nil, fmt.Errorf("semantic graph mutation lacks its deferred graph source")
	}
	graphDrafts, err := state.activateObligationGraph(mutation.version, source)
	if err != nil {
		return nil, err
	}
	for index := range graphDrafts {
		graphDrafts[index] = graphDrafts[index].withSource(source)
	}
	return append(drafts, graphDrafts...), nil
}

func (state *semanticReplayState) deferredSourceForGraph(
	version uint64,
) (cognitionreplay.SourceRecord, bool) {
	for _, source := range state.deferredSources {
		if source.Kind == "obligation_graph" && uint64(source.Sequence) == version {
			return source, true
		}
	}
	return cognitionreplay.SourceRecord{}, false
}

func cloneSemanticProgress(value cognitionruntime.EpisodeProgress) cognitionruntime.EpisodeProgress {
	value.ObligationGraph = value.ObligationGraph.Clone()
	if value.Completion != nil {
		completion := value.Completion.Clone()
		value.Completion = &completion
	}
	if value.Cancellation != nil {
		cancellation := *value.Cancellation
		value.Cancellation = &cancellation
	}
	return value
}

func semanticObligationKnowledgeStatus(status cognition.ObligationStatus) cognitionreplay.KnowledgeStatus {
	switch status {
	case cognition.ObligationProposed:
		return cognitionreplay.KnowledgePending
	case cognition.ObligationReady:
		return cognitionreplay.KnowledgeReady
	case cognition.ObligationBlocked:
		return cognitionreplay.KnowledgeBlocked
	case cognition.ObligationActive:
		return cognitionreplay.KnowledgeActive
	case cognition.ObligationSatisfied:
		return cognitionreplay.KnowledgeSatisfied
	case cognition.ObligationFailed:
		return cognitionreplay.KnowledgeFailed
	case cognition.ObligationSuperseded:
		return cognitionreplay.KnowledgeSuperseded
	default:
		return cognitionreplay.KnowledgeFailed
	}
}
