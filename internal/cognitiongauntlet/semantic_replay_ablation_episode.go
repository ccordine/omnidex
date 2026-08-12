package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

type ablationEpisodeTraceCursor struct {
	entries []TraceEntry
	index   int
}

func (cursor *ablationEpisodeTraceCursor) next(kind TraceKind) (TraceEntry, error) {
	if cursor.index >= len(cursor.entries) {
		return TraceEntry{}, fmt.Errorf("sealed ablation trace ended before %s", kind)
	}
	entry := cursor.entries[cursor.index]
	if entry.Kind != kind {
		return TraceEntry{}, fmt.Errorf(
			"sealed ablation trace entry %d is %s, want %s",
			cursor.index+1, entry.Kind, kind,
		)
	}
	cursor.index++
	return entry, nil
}

func verifyAblationSemanticEpisodeTrace(
	episode SealedEpisode,
	evidence ablationEvidenceArtifact,
) error {
	root := evidence.Root
	cursor := &ablationEpisodeTraceCursor{entries: episode.Manifest.Trace}
	bootstrapEntry, err := cursor.next(TraceProviderBootstrap)
	if err != nil {
		return err
	}
	bootstrap, err := decodeRuntimeBrainBootstrapTrace(bootstrapEntry)
	if err != nil || bootstrap != root.BrainBootstrap {
		return fmt.Errorf("sealed ablation Brain bootstrap differs from evidence: %v", err)
	}
	activationEntry, err := cursor.next(TraceProviderActivation)
	if err != nil {
		return err
	}
	activation, err := decodeRuntimeProviderActivationTrace(activationEntry)
	if err != nil || activation != root.ProviderActivation {
		return fmt.Errorf("sealed ablation provider activation differs from evidence: %v", err)
	}
	if err := consumeAblationTraceObservations(cursor, root.Transitions[0]); err != nil {
		return err
	}
	actions, _, err := indexAblationActionEvidence(root)
	if err != nil {
		return err
	}
	for index, call := range root.Calls {
		projectionEntry, err := cursor.next(TraceProjection)
		if err != nil {
			return err
		}
		outcomeEntry, err := cursor.next(ablationCallTraceKind(call))
		if err != nil {
			return err
		}
		if err := verifyAblationSemanticEpisodeCall(
			projectionEntry, outcomeEntry, call,
		); err != nil {
			return fmt.Errorf("sealed ablation call %d: %w", index+1, err)
		}
		action, exists := actions[uint32(index+1)]
		if !exists {
			continue
		}
		actionEntry, err := cursor.next(TraceAction)
		if err != nil {
			return err
		}
		actual, err := decodeActionTrace(
			actionEntry, cognition.EpisodeRef{ID: root.EpisodeID},
		)
		if err != nil || !reflect.DeepEqual(actual, action.Trace) {
			return fmt.Errorf("sealed ablation action %d differs from evidence: %v", index+1, err)
		}
		if action.Trace.Transition != nil {
			if err := consumeAblationTraceObservations(cursor, *action.Trace.Transition); err != nil {
				return err
			}
		}
	}
	if err := consumeAblationTraceFailure(cursor, evidence); err != nil {
		return err
	}
	evidenceEntry, err := cursor.next(TraceAblationEvidence)
	if err != nil {
		return err
	}
	evidenceAuthority, err := decodeAblationEvidenceTrace(evidenceEntry)
	wantAuthority, authorityErr := ablationEvidenceAuthorityFromEpisode(episode)
	if err != nil || authorityErr != nil || evidenceAuthority != wantAuthority {
		return fmt.Errorf("sealed ablation evidence trace changed: %v %v", err, authorityErr)
	}
	terminalEntry, err := cursor.next(TraceTerminal)
	if err != nil {
		return err
	}
	if err := verifyAblationSemanticEpisodeTerminal(terminalEntry, root); err != nil {
		return err
	}
	if cursor.index != len(cursor.entries) {
		return fmt.Errorf("sealed ablation trace contains %d unconsumed entries", len(cursor.entries)-cursor.index)
	}
	return nil
}

func ablationCallTraceKind(call ablationCallEvidence) TraceKind {
	if call.Result.ProviderRequestDisposition == "dispatched" {
		return TraceModelCall
	}
	return TracePolicyDisposition
}

func verifyAblationSemanticEpisodeCall(
	projectionEntry TraceEntry,
	outcomeEntry TraceEntry,
	call ablationCallEvidence,
) error {
	wantProjection := newAblationProjectionTrace(call.Projection.Projection)
	var gotProjection ProjectionTrace
	if projectionEntry.ID != call.Projection.Projection.ID || projectionEntry.Revision != nil ||
		decodeTracePayload(projectionEntry.Payload, &gotProjection, "ablation projection trace") != nil ||
		!reflect.DeepEqual(gotProjection, wantProjection) {
		return fmt.Errorf("Context Projection differs from exact call input")
	}
	wantOutcome, err := newAblationPolicyTracePayload(
		call.Projection.Projection, ablationCallFromEvidence(call),
	)
	if err != nil || outcomeEntry.Kind != wantOutcome.kind ||
		outcomeEntry.ID != call.Attempt.ID || outcomeEntry.Revision != nil ||
		!reflect.DeepEqual(outcomeEntry.Payload, wantOutcome.payload) {
		return fmt.Errorf("policy outcome differs from exact call evidence: %v", err)
	}
	return nil
}

func consumeAblationTraceObservations(
	cursor *ablationEpisodeTraceCursor,
	transition cognition.Transition,
) error {
	for index, observation := range transition.Observations {
		entry, err := cursor.next(TraceObservation)
		if err != nil {
			return err
		}
		var actual cognition.Observation
		if entry.ID != string(observation.ID) || entry.Revision == nil ||
			*entry.Revision != observation.Revision ||
			decodeTracePayload(entry.Payload, &actual, "ablation observation trace") != nil ||
			!reflect.DeepEqual(actual, observation) {
			return fmt.Errorf("sealed ablation observation %d differs from evidence", index+1)
		}
	}
	return nil
}

func verifyAblationSemanticEpisodeTerminal(
	entry TraceEntry,
	root ablationEvidenceRoot,
) error {
	want := ablationTerminalRecord{
		Revision: root.Terminal.Revision, PublicOutcome: root.Terminal.PublicOutcome,
		GoalSatisfied: root.Terminal.GoalSatisfied,
	}
	var actual ablationTerminalRecord
	if entry.ID != "terminal-"+root.Terminal.Revision.SHA256 || entry.Revision == nil ||
		*entry.Revision != root.Terminal.Revision ||
		decodeTracePayload(entry.Payload, &actual, "ablation terminal trace") != nil ||
		actual != want {
		return fmt.Errorf("sealed ablation terminal differs from exact evidence")
	}
	return nil
}
