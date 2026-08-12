package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workingset"
)

type semanticProjectionRecord struct {
	projection  contextbuilder.Projection
	callOrdinal int64
}

type semanticSnapshotRecord struct {
	snapshot      cognition.RuntimeSnapshot
	preparationID string
	callOrdinal   int64
}

type semanticProgressCommandRecord struct {
	command     cognitionruntime.CompletionCommand
	callOrdinal int64
	sequence    int64
}

type semanticGraphMutation struct {
	version           uint64
	kind              string
	applicationSource uint64
}

type semanticActivationBootstrap struct {
	trace    queue.CognitionBrainBootstrapTrace
	sequence int64
}

type semanticActivationFailure struct {
	record    queue.CognitionSealedTraceRecord
	bootstrap cognitionpolicy.BrainBootstrap
	failure   cognitionpolicy.ProviderProcessFailure
}

type semanticReplayState struct {
	trace                      productionTrace
	sources                    []cognitionreplay.SourceRecord
	sourceBlobs                map[string]cognitionreplay.Blob
	events                     []cognitionreplay.Event
	eventBlobs                 []cognitionreplay.Blob
	entries                    map[string]cognitionreplay.KnowledgeEntry
	checkpoints                []cognitionreplay.KnowledgeCheckpoint
	frozenBrain                cognitionpolicy.AttestedBrain
	goal                       cognition.GoalExpression
	completionAuthority        cognition.CompletionAuthority
	actionCatalog              cognition.ActionCatalog
	initialBudget              cognition.RuntimeBudget
	attempts                   map[string]cognitionpolicy.CallAttempt
	attemptOrdinals            map[int64]string
	attemptSourceSHA           map[string]string
	results                    map[string]cognitionpolicy.CallResult
	abandoned                  map[string]struct{}
	policyTimings              map[string]queue.CognitionTracePolicyTiming
	decisions                  map[string]cognition.CognitionDecision
	projections                map[cognition.ContextProjectionID]semanticProjectionRecord
	projectionConsumers        map[cognition.ContextProjectionID]string
	snapshots                  map[string]semanticSnapshotRecord
	snapshotConsumers          map[string]string
	evidence                   semanticReplaySupplement
	usedPolicyEvidence         map[string]struct{}
	actions                    map[cognition.ActionID]queue.CognitionTraceAction
	callActions                map[string]cognition.ActionID
	actionOrdinals             map[cognition.ActionID]int64
	transitions                map[cognition.ActionID]cognition.Transition
	transitionRecords          map[string]cognition.Transition
	acceptedFactTransitions    map[string]struct{}
	acceptedFacts              map[string]queue.CognitionAcceptedFactMaterializationMember
	initialFactScope           cognition.ObligationID
	actionEvents               map[cognition.ActionID]semanticActionLifecycle
	observations               map[cognition.ObservationID]cognition.EvidenceRef
	latestRevision             *cognition.WorldRevision
	worldTerminal              bool
	worldPublicOutcome         string
	reconciles                 map[string]cognitionruntime.ReconciliationCommand
	reconciliationOrdinals     map[int64]string
	reconciliationReceipts     map[string]cognitionruntime.ReconciliationReceipt
	reconciliationCalls        map[string]string
	callReconciliations        map[string]string
	proposalMaterializations   map[string][]queue.CognitionProposalMaterializationTraceMember
	recoveries                 map[string]queue.CognitionTraceAcceptedDecisionRecovery
	recoveryConsumers          map[string]string
	progressCommands           map[string]semanticProgressCommandRecord
	progressResults            map[string]cognitionruntime.EpisodeProgress
	graphs                     map[uint64]cognition.ObligationGraphSnapshot
	graphRecordIDs             map[uint64]string
	classifiedGraphs           map[uint64]string
	graphMutations             map[string]semanticGraphMutation
	activeGraphVersion         uint64
	deferredSources            map[uint64]cognitionreplay.SourceRecord
	consumedDeferredSources    map[uint64]struct{}
	obligations                map[cognition.ObligationID]cognition.Obligation
	terminalProgress           *cognitionruntime.EpisodeProgress
	terminalProgressCommandID  string
	cancellation               *cognitionruntime.CancellationEvidence
	lifecycleRetirement        *queue.CognitionSealedTraceRecord
	initialActor               cognition.AttemptRef
	initialBootstrapTrace      *queue.CognitionBrainBootstrapTrace
	initialBrainBootstrap      bool
	initialProviderObservation bool
	providerProcesses          map[int64]cognitionpolicy.ProviderProcessObservation
	replayBootstraps           map[string]queue.CognitionBrainBootstrapTrace
	activationBootstraps       map[string]semanticActivationBootstrap
	activationFailures         map[string]semanticActivationFailure
	workingSet                 *workingset.Set
	workingSetProjectionPoints []queue.CognitionProjectionWorkingVersion
	workingSetTupleErr         error
	workingSetTerminal         bool
	started                    bool
	terminal                   bool
}

func newSemanticReplayState(
	trace productionTrace,
	sources []cognitionreplay.SourceRecord,
	blobs []cognitionreplay.Blob,
	frozen cognitionpolicy.AttestedBrain,
	goal cognition.GoalExpression,
	completion cognition.CompletionAuthority,
	catalog cognition.ActionCatalog,
	budget cognition.RuntimeBudget,
	evidence semanticReplaySupplement,
) *semanticReplayState {
	byDigest := make(map[string]cognitionreplay.Blob, len(blobs))
	for _, blob := range blobs {
		byDigest[blob.SHA256] = blob
	}
	workingSetPoints, workingSetTupleErr := semanticWorkingSetProjectionPoints(trace.Records)
	empty := cognitionreplay.KnowledgeState{
		Schema:  cognitionreplay.KnowledgeStateSchemaV1,
		Entries: []cognitionreplay.KnowledgeEntry{},
	}
	return &semanticReplayState{
		trace: trace, sources: sources, sourceBlobs: byDigest,
		entries: make(map[string]cognitionreplay.KnowledgeEntry), frozenBrain: frozen,
		goal: goal, completionAuthority: completion, actionCatalog: catalog,
		initialBudget:              budget,
		attempts:                   make(map[string]cognitionpolicy.CallAttempt),
		attemptOrdinals:            make(map[int64]string),
		attemptSourceSHA:           make(map[string]string),
		results:                    make(map[string]cognitionpolicy.CallResult),
		abandoned:                  make(map[string]struct{}),
		policyTimings:              make(map[string]queue.CognitionTracePolicyTiming),
		decisions:                  make(map[string]cognition.CognitionDecision),
		projections:                make(map[cognition.ContextProjectionID]semanticProjectionRecord),
		projectionConsumers:        make(map[cognition.ContextProjectionID]string),
		snapshots:                  make(map[string]semanticSnapshotRecord),
		snapshotConsumers:          make(map[string]string),
		evidence:                   evidence,
		usedPolicyEvidence:         make(map[string]struct{}),
		actions:                    make(map[cognition.ActionID]queue.CognitionTraceAction),
		callActions:                make(map[string]cognition.ActionID),
		actionOrdinals:             make(map[cognition.ActionID]int64),
		transitions:                make(map[cognition.ActionID]cognition.Transition),
		transitionRecords:          make(map[string]cognition.Transition),
		acceptedFactTransitions:    make(map[string]struct{}),
		acceptedFacts:              make(map[string]queue.CognitionAcceptedFactMaterializationMember),
		actionEvents:               make(map[cognition.ActionID]semanticActionLifecycle),
		observations:               make(map[cognition.ObservationID]cognition.EvidenceRef),
		reconciles:                 make(map[string]cognitionruntime.ReconciliationCommand),
		reconciliationOrdinals:     make(map[int64]string),
		reconciliationReceipts:     make(map[string]cognitionruntime.ReconciliationReceipt),
		reconciliationCalls:        make(map[string]string),
		callReconciliations:        make(map[string]string),
		proposalMaterializations:   make(map[string][]queue.CognitionProposalMaterializationTraceMember),
		recoveries:                 make(map[string]queue.CognitionTraceAcceptedDecisionRecovery),
		recoveryConsumers:          make(map[string]string),
		progressCommands:           make(map[string]semanticProgressCommandRecord),
		progressResults:            make(map[string]cognitionruntime.EpisodeProgress),
		graphs:                     make(map[uint64]cognition.ObligationGraphSnapshot),
		graphRecordIDs:             make(map[uint64]string),
		classifiedGraphs:           make(map[uint64]string),
		graphMutations:             make(map[string]semanticGraphMutation),
		deferredSources:            make(map[uint64]cognitionreplay.SourceRecord),
		consumedDeferredSources:    make(map[uint64]struct{}),
		obligations:                make(map[cognition.ObligationID]cognition.Obligation),
		activationBootstraps:       make(map[string]semanticActivationBootstrap),
		activationFailures:         make(map[string]semanticActivationFailure),
		providerProcesses:          make(map[int64]cognitionpolicy.ProviderProcessObservation),
		replayBootstraps:           make(map[string]queue.CognitionBrainBootstrapTrace),
		workingSetProjectionPoints: workingSetPoints,
		workingSetTupleErr:         workingSetTupleErr,
		checkpoints: []cognitionreplay.KnowledgeCheckpoint{{
			Sequence: 1, AfterEvent: 0, State: empty,
		}},
	}
}

func (state *semanticReplayState) accept(
	index int,
	record queue.CognitionSealedTraceRecord,
) error {
	if index < 0 || index >= len(state.sources) {
		return fmt.Errorf("semantic replay source index is invalid")
	}
	source := state.sources[index]
	drafts, err := state.mapRecord(record, source)
	if err != nil {
		return fmt.Errorf("map sealed cognition source %s/%s: %w", record.Kind, record.ID, err)
	}
	if len(drafts) == 0 {
		if deferred, exists := state.deferredSources[source.Ordinal]; !exists || deferred != source {
			return fmt.Errorf("sealed cognition source %s/%s has no semantic event", record.Kind, record.ID)
		}
		return nil
	}
	for _, draft := range drafts {
		if err := state.appendEvent(draft, source); err != nil {
			return err
		}
	}
	return nil
}

func (state *semanticReplayState) deferSource(source cognitionreplay.SourceRecord) error {
	if _, duplicate := state.deferredSources[source.Ordinal]; duplicate {
		return fmt.Errorf("semantic source %d is deferred twice", source.Ordinal)
	}
	state.deferredSources[source.Ordinal] = source
	return nil
}

func (state *semanticReplayState) appendEvent(
	draft semanticEventDraft,
	source cognitionreplay.SourceRecord,
) error {
	if draft.Source != nil {
		source = *draft.Source
		deferred, exists := state.deferredSources[source.Ordinal]
		if !exists || deferred != source {
			return fmt.Errorf("semantic event cites an undeferred source")
		}
		state.consumedDeferredSources[source.Ordinal] = struct{}{}
	}
	sequence := uint64(len(state.events) + 1)
	event := cognitionreplay.Event{
		Sequence: sequence, Kind: draft.Kind,
		MappingSchema: cognitionreplay.SemanticMappingSchemaV1,
		Revision:      draft.Revision, Sources: []cognitionreplay.SourceRef{source.Ref()},
		Payload: draft.Payload,
	}
	state.events = append(state.events, event)
	state.applyKnowledge(event, draft.Knowledge)
	for _, change := range draft.KnowledgeChanges {
		state.applyKnowledge(event, change)
	}
	if len(state.events)%100 == 0 {
		state.appendCheckpoint()
	}
	return nil
}

func (state *semanticReplayState) applyKnowledge(
	event cognitionreplay.Event,
	change *semanticKnowledgeChange,
) {
	if change == nil {
		return
	}
	key := string(change.Kind) + "\x00" + change.Ref
	provenance := []uint64{event.Sequence}
	if previous, exists := state.entries[key]; exists {
		provenance = append(append([]uint64(nil), previous.SourceEvents...), event.Sequence)
	}
	entry := cognitionreplay.KnowledgeEntry{
		Kind: change.Kind, Ref: change.Ref, Status: change.Status,
		Authority: change.Authority, Content: event.Payload,
		SourceEvents: provenance,
	}
	state.entries[key] = entry
}
