package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/queue"
)

func (*semanticReplayState) mapBeliefRevision(
	queue.CognitionSealedTraceRecord,
	cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	// A belief revision names the durable Task Ledger entry, while the current
	// frozen trace records only the earlier model proposal. Without the exact
	// code-owned proposal materialization descriptor there is no portable proof
	// that both identities are the same belief. Refuse to mint a split lifecycle.
	return nil, fmt.Errorf(
		"semantic belief revision requires frozen proposal-materialization evidence",
	)
}

func (state *semanticReplayState) mapPlanRevision(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognition.PlanRevisionMaterialization
	if err := decodeProductionPayload(record.Payload, &value, "semantic plan revision"); err != nil ||
		value.Validate() != nil || value.ID != record.ID ||
		value.EpisodeID != state.trace.Header.EpisodeID || record.CallOrdinal < 1 ||
		record.Phase != 44 || record.Sequence < 2 {
		return nil, fmt.Errorf("invalid semantic plan revision: %v", err)
	}
	version := uint64(record.Sequence)
	if version != state.activeGraphVersion+1 {
		return nil, fmt.Errorf("semantic plan revision is not the next active graph mutation")
	}
	before, beforeExists := state.graphs[version-1]
	after, afterExists := state.graphs[version]
	snapshot, snapshotExists := state.snapshots[value.SourceSnapshotSHA256]
	callID := state.attemptOrdinals[record.CallOrdinal]
	decision, decisionExists := state.decisions[callID]
	if !beforeExists || !afterExists || !snapshotExists || !decisionExists ||
		snapshot.callOrdinal != record.CallOrdinal || state.graphRecordIDs[version] != value.ID ||
		before.SHA256 != value.ExpectedGraphSHA256 || after.SHA256 != value.ResultGraphSHA256 {
		return nil, fmt.Errorf("semantic plan revision lacks its exact source and graph authorities")
	}
	schema, exists := snapshot.snapshot.ActionCatalog().Schema(decision.Action.Kind)
	if !exists {
		return nil, fmt.Errorf("semantic plan revision action schema is unavailable")
	}
	rederived, err := cognitionstate.MaterializePlanRevisionProposal(
		cognitionstate.PlanRevisionProposalInput{
			Graph: before, Snapshot: snapshot.snapshot, Decision: decision,
			ActionSchema: schema, ProposalIndex: value.ProposalIndex,
			CompletionAuthority: state.completionAuthority,
		},
	)
	if err != nil || !reflect.DeepEqual(rederived, value) {
		return nil, fmt.Errorf("semantic plan revision differs from exact code materialization: %v", err)
	}
	applied, err := value.Apply(before)
	if err != nil || !reflect.DeepEqual(applied, after) {
		return nil, fmt.Errorf("semantic plan revision output graph differs: %v", err)
	}
	if _, classified := state.classifiedGraphs[version]; classified {
		return nil, fmt.Errorf("semantic obligation graph has duplicate mutation authority")
	}
	reconciliationID := state.reconciliationOrdinals[record.CallOrdinal]
	if reconciliationID == "" {
		return nil, fmt.Errorf("semantic plan revision lacks its reconciliation")
	}
	state.classifiedGraphs[version] = "plan_revision"
	if _, duplicate := state.graphMutations[reconciliationID]; duplicate {
		return nil, fmt.Errorf("semantic reconciliation has two graph mutations")
	}
	if err := state.deferSource(source); err != nil {
		return nil, err
	}
	state.graphMutations[reconciliationID] = semanticGraphMutation{
		version: version, kind: "plan_revision", applicationSource: source.Ordinal,
	}
	return nil, nil
}

func (state *semanticReplayState) mapContextProjection(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value contextbuilder.Projection
	if err := decodeProductionPayload(record.Payload, &value, "semantic Context Projection"); err != nil ||
		value.Validate() != nil || string(value.ID) != record.ID ||
		record.CallOrdinal < 1 || record.Phase != 10 || record.Sequence != record.CallOrdinal {
		return nil, fmt.Errorf("invalid semantic Context Projection: %v", err)
	}
	projectionID := cognition.ContextProjectionID(value.ID)
	if _, duplicate := state.projections[projectionID]; duplicate {
		return nil, fmt.Errorf("semantic Context Projection is duplicated")
	}
	if state.workingSet == nil || state.workingSetTerminal ||
		string(value.WorkingSetID) != string(state.workingSet.ID()) ||
		value.WorkingSetVersion != state.workingSet.Version() ||
		verifySemanticProjectionWorkingSet(value, state.workingSet) != nil {
		return nil, fmt.Errorf("semantic Context Projection differs from the replayed Working Set")
	}
	state.projections[projectionID] = semanticProjectionRecord{
		projection: value, callOrdinal: record.CallOrdinal,
	}
	draft := sourceDraft(cognitionreplay.EventContextProjected, source)
	draft.Knowledge = knowledgeChange(
		cognitionreplay.KnowledgeProjection, "projection://"+record.ID,
		cognitionreplay.KnowledgeActive, cognitionreplay.AuthorityCode,
	)
	return []semanticEventDraft{draft}, nil
}

type semanticRuntimeSnapshot struct {
	Goal              cognition.GoalExpression       `json:"goal"`
	CurrentRevision   cognition.WorldRevision        `json:"current_revision"`
	CurrentObligation cognition.Obligation           `json:"current_obligation"`
	ActionCatalog     cognition.ActionCatalog        `json:"action_catalog"`
	Attempt           cognition.AttemptRef           `json:"attempt"`
	ContextProjection cognition.ContextProjectionRef `json:"context_projection"`
	Budget            cognition.RuntimeBudget        `json:"budget"`
	EvidenceRefs      []cognition.EvidenceRef        `json:"evidence_refs"`
}

func (state *semanticReplayState) mapRuntimeSnapshot(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value semanticRuntimeSnapshot
	if err := decodeProductionPayload(record.Payload, &value, "semantic runtime snapshot"); err != nil {
		return nil, err
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		value.Goal, value.CurrentRevision, value.CurrentObligation, value.ActionCatalog,
		value.Attempt, value.ContextProjection, value.Budget, value.EvidenceRefs,
	)
	if err != nil || snapshot.SHA256() != record.SHA256 ||
		record.ID != "cognition_snapshot_"+record.SHA256 ||
		record.CallOrdinal < 1 || record.Phase != 20 || record.Sequence != record.CallOrdinal ||
		value.CurrentRevision.EpisodeID != state.trace.Header.EpisodeID ||
		state.latestRevision == nil || value.CurrentRevision != *state.latestRevision ||
		!reflect.DeepEqual(value.Goal, state.goal) ||
		!reflect.DeepEqual(value.ActionCatalog, state.actionCatalog) ||
		validateSemanticRuntimeBudget(
			state.initialBudget, value.Budget, record.CallOrdinal, len(state.attempts),
		) != nil {
		return nil, fmt.Errorf("invalid semantic runtime snapshot: %v", err)
	}
	for _, ref := range snapshot.EvidenceRefs() {
		observed, exists := state.observations[ref.ObservationID]
		if !exists || observed != ref || ref.Revision.Number > snapshot.CurrentRevision().Number {
			return nil, fmt.Errorf("semantic runtime snapshot cites unobserved evidence")
		}
	}
	active, graphExists := state.graphs[state.activeGraphVersion]
	if !graphExists || !semanticSnapshotObligationIsActive(value.CurrentObligation, active) {
		return nil, fmt.Errorf("semantic runtime snapshot cites an inactive obligation graph")
	}
	projection, exists := state.projections[value.ContextProjection.ID]
	if !exists || projection.callOrdinal != record.CallOrdinal ||
		semanticContextProjectionRef(projection.projection) != value.ContextProjection {
		return nil, fmt.Errorf("semantic runtime snapshot lacks its exact Context Projection")
	}
	if err := verifySemanticSnapshotEvidenceRefs(
		projection.projection, snapshot.EvidenceRefs(), state.observations,
		state.acceptedFacts, snapshot.CurrentRevision(), active,
	); err != nil {
		return nil, err
	}
	if _, duplicate := state.snapshots[record.SHA256]; duplicate {
		return nil, fmt.Errorf("semantic runtime snapshot identity is duplicated")
	}
	if err := state.consumeProjection(value.ContextProjection.ID, record.SHA256); err != nil {
		return nil, err
	}
	state.snapshots[record.SHA256] = semanticSnapshotRecord{
		snapshot: snapshot, preparationID: record.ID, callOrdinal: record.CallOrdinal,
	}
	draft := sourceKnowledgeDraft(
		cognitionreplay.EventEvidenceAcquired, source,
		cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgeActive,
		cognitionreplay.AuthorityCode,
	)
	draft.Revision = semanticReplayRevision(value.CurrentRevision)
	draft.Knowledge.Ref = "runtime-snapshot://" + record.ID
	return []semanticEventDraft{draft}, nil
}

func validateSemanticRuntimeBudget(
	initial cognition.RuntimeBudget,
	actual cognition.RuntimeBudget,
	callOrdinal int64,
	priorAttempts int,
) error {
	if initial.Validate() != nil || callOrdinal < 1 || priorAttempts < 0 ||
		callOrdinal != int64(priorAttempts)+1 ||
		uint64(priorAttempts) > uint64(initial.RemainingPolicyCalls) {
		return fmt.Errorf("semantic runtime budget call ordinal is not exact")
	}
	want := initial
	want.RemainingPolicyCalls -= uint32(priorAttempts)
	if actual != want {
		return fmt.Errorf("semantic runtime budget differs from the frozen durable call count")
	}
	return nil
}

func semanticSnapshotObligationIsActive(
	current cognition.Obligation,
	graph cognition.ObligationGraphSnapshot,
) bool {
	for _, obligation := range graph.Obligations {
		if obligation.ID == current.ID {
			return reflect.DeepEqual(obligation, current)
		}
	}
	return false
}

func semanticContextProjectionRef(
	value contextbuilder.Projection,
) cognition.ContextProjectionRef {
	return cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(value.ID), SHA256: value.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(value.WorkingSetID),
		WorkingSetVersion: value.WorkingSetVersion, RendererVersion: value.RendererVersion,
	}
}
