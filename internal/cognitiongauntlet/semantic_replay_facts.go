package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapAcceptedFactMaterialization(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	value, err := queue.DecodeCognitionAcceptedFactMaterialization(
		record.Payload, record.SHA256,
	)
	if err != nil {
		return nil, fmt.Errorf("decode semantic accepted-fact materialization: %w", err)
	}
	transition, exists := state.transitionRecords[value.TransitionID]
	if !exists || transition.Current.Number != value.TransitionRevision {
		return nil, fmt.Errorf("semantic accepted-fact materialization lacks its exact transition")
	}
	authority, err := newVisibleObservationFactAuthority()
	if err != nil {
		return nil, fmt.Errorf("construct semantic accepted-fact authority: %w", err)
	}
	trace := queue.CognitionAcceptedFactMaterializationTraceAuthority{
		TransitionID: value.TransitionID, TransitionSHA256: value.TransitionSHA256,
		CallOrdinal: uint64(record.CallOrdinal), Phase: record.Phase,
		Sequence: record.Sequence, ID: record.ID, SHA256: record.SHA256,
	}
	if record.CallOrdinal < 0 || queue.VerifyCognitionAcceptedFactMaterializationTrace(
		value, trace, transition, authority,
	) != nil {
		return nil, fmt.Errorf("semantic accepted-fact materialization changed exact authority")
	}
	if _, duplicate := state.acceptedFactTransitions[value.TransitionID]; duplicate {
		return nil, fmt.Errorf("semantic transition has two accepted-fact materializations")
	}
	if value.ActionID == "" {
		if state.initialFactScope != "" {
			return nil, fmt.Errorf("semantic initial accepted-fact materialization is duplicated")
		}
		state.initialFactScope = value.ScopeObligationID
	} else {
		action, actionExists := state.actions[value.ActionID]
		if !actionExists || action.ObligationID != value.ScopeObligationID ||
			state.actionOrdinals[value.ActionID] != record.CallOrdinal {
			return nil, fmt.Errorf("semantic accepted-fact materialization has changed action scope")
		}
	}
	for _, member := range value.Members {
		if _, duplicate := state.acceptedFacts[member.EntryURI]; duplicate {
			return nil, fmt.Errorf("semantic accepted-fact entry URI is duplicated")
		}
		state.acceptedFacts[member.EntryURI] = member
	}
	state.acceptedFactTransitions[value.TransitionID] = struct{}{}
	drafts := []semanticEventDraft{sourceDraft(cognitionreplay.EventEvidenceAcquired, source)}
	revision := semanticReplayRevision(transition.Current)
	for _, member := range value.Members {
		draft, draftErr := state.typedDraft(
			cognitionreplay.EventFactAccepted, source, member, revision,
			knowledgeChange(
				cognitionreplay.KnowledgeEvidence, member.EntryURI,
				cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityCode,
			),
		)
		if draftErr != nil {
			return nil, fmt.Errorf("derive semantic accepted-fact event: %w", draftErr)
		}
		drafts = append(drafts, draft)
	}
	return drafts, nil
}

func (state *semanticReplayState) finishAcceptedFactMaterializations() error {
	if len(state.acceptedFactTransitions) != len(state.transitionRecords) {
		return fmt.Errorf("semantic transitions lack one exact zero-capable accepted-fact materialization")
	}
	for transitionID := range state.transitionRecords {
		if _, exists := state.acceptedFactTransitions[transitionID]; !exists {
			return fmt.Errorf("semantic transition %q lacks accepted-fact materialization", transitionID)
		}
	}
	initial, exists := state.graphs[1]
	if !exists || state.initialFactScope == "" || state.initialFactScope != initial.RootID {
		return fmt.Errorf("semantic initial accepted-fact materialization has changed root scope")
	}
	return nil
}
