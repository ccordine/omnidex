package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapTransition(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognition.Transition
	if err := decodeProductionPayload(record.Payload, &value, "semantic transition"); err != nil {
		return nil, err
	}
	if err := queue.VerifyCognitionTraceTransitionIdentity(
		state.trace.Header.EpisodeID, record.ID, record.SHA256,
	); err != nil {
		return nil, err
	}
	if _, duplicate := state.transitionRecords[record.ID]; duplicate {
		return nil, fmt.Errorf("semantic world transition identity is duplicated")
	}
	if value.ActionID == "" {
		if err := value.ValidateStart(); err != nil || state.started ||
			value.Current.EpisodeID != state.trace.Header.EpisodeID ||
			record.CallOrdinal != 0 || record.Phase != 10 ||
			record.Sequence != int64(value.Current.Number) {
			return nil, fmt.Errorf("invalid or duplicate world start: %v", err)
		}
		state.started = true
		current := value.Current
		state.latestRevision = &current
	} else {
		action, exists := state.actions[value.ActionID]
		if !exists || state.latestRevision == nil || state.worldTerminal ||
			action.Status != queue.CognitionActionSucceeded || action.ResultRevision == nil ||
			*state.latestRevision != action.ExpectedRevision ||
			*action.ResultRevision != value.Current ||
			state.actionOrdinals[value.ActionID] != record.CallOrdinal ||
			record.Phase != 53 || record.Sequence != int64(value.Current.Number) ||
			value.ValidateApply(
				cognition.EpisodeRef{ID: state.trace.Header.EpisodeID},
				action.ExpectedRevision, action.RegisteredAction,
			) != nil {
			return nil, fmt.Errorf("world transition lacks its exact registered action")
		}
		if _, duplicate := state.transitions[value.ActionID]; duplicate {
			return nil, fmt.Errorf("world transition consumes an action twice")
		}
		state.transitions[value.ActionID] = value.Clone()
		current := value.Current
		state.latestRevision = &current
	}
	for _, observation := range value.Observations {
		if _, duplicate := state.observations[observation.ID]; duplicate {
			return nil, fmt.Errorf("world observation identity %q is reused", observation.ID)
		}
		state.observations[observation.ID] = observation.EvidenceRef()
	}
	if value.Terminal {
		state.worldTerminal = true
	}
	state.worldPublicOutcome = value.PublicOutcome
	state.transitionRecords[record.ID] = value.Clone()
	revision := semanticReplayTransitionRevision(value)
	kind := cognitionreplay.EventWorldTransition
	if value.ActionID == "" {
		kind = cognitionreplay.EventWorldStarted
	}
	drafts := []semanticEventDraft{{Kind: kind, Revision: revision, Payload: source.Payload}}
	for _, observation := range value.Observations {
		change := knowledgeChange(
			cognitionreplay.KnowledgeObservation, "observation://"+string(observation.ID),
			cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityEnvironment,
		)
		var err error
		drafts, err = appendTypedDraft(
			state, drafts, cognitionreplay.EventObservationAcquired, source,
			observation, revision, change,
		)
		if err != nil {
			return nil, err
		}
	}
	for _, effect := range value.Effects {
		ref := fmt.Sprintf("effect://%s/%s", effect.Kind, effect.ContentSHA256)
		change := knowledgeChange(
			cognitionreplay.KnowledgeEvidence, ref, cognitionreplay.KnowledgeActive,
			cognitionreplay.AuthorityEnvironment,
		)
		var err error
		drafts, err = appendTypedDraft(
			state, drafts, cognitionreplay.EventEvidenceAcquired, source,
			effect, revision, change,
		)
		if err != nil {
			return nil, err
		}
	}
	if value.ActionID != "" {
		action := state.actions[value.ActionID]
		graphDrafts, err := state.activateReconciliationGraph(
			action.ReconciliationID, value.Terminal,
		)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, graphDrafts...)
	}
	return drafts, nil
}

func (state *semanticReplayState) mapAction(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value queue.CognitionTraceAction
	if err := decodeProductionPayload(record.Payload, &value, "semantic action"); err != nil ||
		queue.VerifyCognitionTraceActionIdentity(value) != nil ||
		string(value.RegisteredAction.ID) != record.ID ||
		value.EpisodeID != state.trace.Header.EpisodeID || record.CallOrdinal < 1 ||
		record.Phase != 50 || record.Sequence != 0 || state.latestRevision == nil ||
		state.worldTerminal || value.ExpectedRevision != *state.latestRevision {
		return nil, fmt.Errorf("invalid semantic action: %v", err)
	}
	snapshot, exists := state.snapshots[value.SnapshotSHA256]
	command, commandExists := state.reconciles[value.ReconciliationID]
	receipt, receiptExists := state.reconciliationReceipts[value.ReconciliationID]
	decision, decisionExists := state.decisions[value.PolicyCallID]
	actorBound := commandExists && command.Binding.Attempt == value.RegisteredAction.Actor
	if commandExists && !actorBound {
		actorBound = state.consumeRecoveryForAction(value, command)
	}
	if !exists || snapshot.callOrdinal != record.CallOrdinal ||
		!commandExists || !receiptExists || !decisionExists ||
		state.reconciliationCalls[value.ReconciliationID] != value.PolicyCallID ||
		value.ContextProjection != snapshot.snapshot.ContextProjection() ||
		value.ExpectedRevision != snapshot.snapshot.CurrentRevision() ||
		value.ObligationID != snapshot.snapshot.CurrentObligation().ID ||
		command.SnapshotSHA256 != value.SnapshotSHA256 ||
		command.Projection != value.ContextProjection ||
		!actorBound ||
		!reflect.DeepEqual(command.Decision, decision) ||
		!reflect.DeepEqual(value.Decision, decision) ||
		!semanticEvidenceRefsAvailable(value.Decision.EvidenceRefs, snapshot.snapshot, state.observations) ||
		receipt.SHA256 != value.ReconciliationSHA256 ||
		receipt.ActionSchema != value.SchemaRef {
		return nil, fmt.Errorf("semantic action differs from its accepted decision and reconciliation")
	}
	if _, duplicate := state.actions[value.RegisteredAction.ID]; duplicate {
		return nil, fmt.Errorf("semantic action is duplicated")
	}
	if prior, duplicate := state.callActions[value.PolicyCallID]; duplicate {
		return nil, fmt.Errorf("semantic accepted call already selected action %q", prior)
	}
	state.actions[value.RegisteredAction.ID] = value
	state.callActions[value.PolicyCallID] = value.RegisteredAction.ID
	state.actionOrdinals[value.RegisteredAction.ID] = record.CallOrdinal
	drafts := []semanticEventDraft{{
		Kind:     cognitionreplay.EventActionSelected,
		Revision: semanticReplayRevision(value.ExpectedRevision), Payload: source.Payload,
	}}
	if value.Status == queue.CognitionActionFailed {
		drafts = append(drafts, semanticEventDraft{
			Kind:     cognitionreplay.EventFailureRecorded,
			Revision: semanticReplayRevision(value.ExpectedRevision), Payload: source.Payload,
			Knowledge: knowledgeChange(
				cognitionreplay.KnowledgeFailure, "action-failure://"+record.ID,
				cognitionreplay.KnowledgeFailed, cognitionreplay.AuthorityEnvironment,
			),
		})
	}
	return drafts, nil
}

func (state *semanticReplayState) consumeRecoveryForAction(
	action queue.CognitionTraceAction,
	command cognitionruntime.ReconciliationCommand,
) bool {
	for recoveryID, recovery := range state.recoveries {
		if recovery.Recovery.PolicyCallID != action.PolicyCallID ||
			recovery.SnapshotSHA256 != action.SnapshotSHA256 ||
			recovery.Binding.Attempt != action.RegisteredAction.Actor ||
			recovery.SourceActor != command.Binding.Attempt || recovery.ActionSchema != action.SchemaRef {
			continue
		}
		if _, used := state.recoveryConsumers[recoveryID]; used {
			return false
		}
		state.recoveryConsumers[recoveryID] = string(action.RegisteredAction.ID)
		return true
	}
	return false
}

func semanticEvidenceRefsAvailable(
	refs []cognition.EvidenceRef,
	snapshot cognition.RuntimeSnapshot,
	observations map[cognition.ObservationID]cognition.EvidenceRef,
) bool {
	available := make(map[cognition.EvidenceRef]struct{}, len(snapshot.EvidenceRefs()))
	for _, ref := range snapshot.EvidenceRefs() {
		available[ref] = struct{}{}
	}
	for _, ref := range refs {
		observed, exists := observations[ref.ObservationID]
		if !exists || observed != ref || ref.Revision.Number > snapshot.CurrentRevision().Number {
			return false
		}
		if _, projected := available[ref]; !projected {
			return false
		}
	}
	return true
}
