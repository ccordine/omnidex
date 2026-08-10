package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workingset"
)

type productionTraceDiagnostics struct {
	timings    map[string]queue.CognitionTracePolicyTiming
	recoveries map[string]queue.CognitionTraceAcceptedDecisionRecovery
	start      *queue.CognitionTraceWorkingSetSnapshot
	final      *queue.CognitionTraceWorkingSetSnapshot
	events     []queue.CognitionTraceWorkingSetEvent
}

func newProductionTraceDiagnostics() productionTraceDiagnostics {
	return productionTraceDiagnostics{
		timings:    make(map[string]queue.CognitionTracePolicyTiming),
		recoveries: make(map[string]queue.CognitionTraceAcceptedDecisionRecovery),
	}
}

func (diagnostics *productionTraceDiagnostics) accept(
	record queue.CognitionSealedTraceRecord,
	header queue.CognitionSealedTracePage,
) error {
	switch record.Kind {
	case "policy_timing":
		return diagnostics.acceptTiming(record, header)
	case "working_set_snapshot":
		return diagnostics.acceptWorkingSetSnapshot(record, header)
	case "working_set_event":
		return diagnostics.acceptWorkingSetEvent(record, header)
	case "accepted_decision_recovery":
		return diagnostics.acceptRecovery(record, header)
	default:
		return fmt.Errorf("sealed production diagnostic kind %q is not registered", record.Kind)
	}
}

func (diagnostics *productionTraceDiagnostics) acceptTiming(
	record queue.CognitionSealedTraceRecord,
	header queue.CognitionSealedTracePage,
) error {
	var timing queue.CognitionTracePolicyTiming
	if err := decodeProductionPayload(record.Payload, &timing, "policy timing"); err != nil {
		return err
	}
	if err := timing.Validate(); err != nil {
		return err
	}
	if record.ID != timing.CallID+":timing" || timing.StartedAt.Before(header.EpisodeStartedAt) ||
		(timing.FinishedAt != nil && timing.FinishedAt.After(header.SealedAt)) {
		return fmt.Errorf("sealed production policy timing changed its durable authority")
	}
	if _, duplicate := diagnostics.timings[timing.CallID]; duplicate {
		return fmt.Errorf("sealed production policy timing is duplicated")
	}
	diagnostics.timings[timing.CallID] = timing
	return nil
}

func (diagnostics *productionTraceDiagnostics) acceptRecovery(
	record queue.CognitionSealedTraceRecord,
	header queue.CognitionSealedTracePage,
) error {
	var recovery queue.CognitionTraceAcceptedDecisionRecovery
	if err := decodeProductionPayload(record.Payload, &recovery, "accepted decision recovery"); err != nil {
		return err
	}
	if err := recovery.Validate(); err != nil {
		return err
	}
	if record.ID != recovery.Recovery.ID || recovery.Binding.Episode.ID != header.EpisodeID ||
		recovery.CreatedAt.Before(header.EpisodeStartedAt) || recovery.CreatedAt.After(header.SealedAt) {
		return fmt.Errorf("sealed production accepted decision recovery changed its authority")
	}
	if _, duplicate := diagnostics.recoveries[recovery.Recovery.ID]; duplicate {
		return fmt.Errorf("sealed production accepted decision recovery is duplicated")
	}
	diagnostics.recoveries[recovery.Recovery.ID] = recovery
	return nil
}

func (diagnostics *productionTraceDiagnostics) acceptWorkingSetSnapshot(
	record queue.CognitionSealedTraceRecord,
	header queue.CognitionSealedTracePage,
) error {
	var snapshot queue.CognitionTraceWorkingSetSnapshot
	if err := decodeProductionPayload(record.Payload, &snapshot, "Working Set snapshot"); err != nil {
		return err
	}
	if snapshot.Schema != queue.CognitionTraceWorkingSetSnapshotSchemaV1 ||
		workingset.ValidateSnapshot(snapshot.Snapshot) != nil ||
		snapshot.CapturedAt.Before(header.EpisodeStartedAt) || snapshot.CapturedAt.After(header.SealedAt) {
		return fmt.Errorf("sealed production Working Set snapshot is invalid")
	}
	switch snapshot.Point {
	case "episode_start":
		if diagnostics.start != nil || record.ID != string(snapshot.Snapshot.ID)+":episode-start" {
			return fmt.Errorf("sealed production Working Set start snapshot is duplicated or changed")
		}
		copy := snapshot
		diagnostics.start = &copy
	case "terminal":
		if diagnostics.final != nil || record.ID != string(snapshot.Snapshot.ID)+":terminal" {
			return fmt.Errorf("sealed production Working Set terminal snapshot is duplicated or changed")
		}
		copy := snapshot
		diagnostics.final = &copy
	default:
		return fmt.Errorf("sealed production Working Set snapshot point %q is not registered", snapshot.Point)
	}
	return nil
}

func (diagnostics *productionTraceDiagnostics) acceptWorkingSetEvent(
	record queue.CognitionSealedTraceRecord,
	header queue.CognitionSealedTracePage,
) error {
	var event queue.CognitionTraceWorkingSetEvent
	if err := decodeProductionPayload(record.Payload, &event, "Working Set event"); err != nil {
		return err
	}
	if event.Schema != queue.CognitionTraceWorkingSetEventSchemaV1 ||
		workingset.ValidateEvent(event.Event) != nil ||
		event.CreatedAt.Before(header.EpisodeStartedAt) || event.CreatedAt.After(header.SealedAt) ||
		record.ID != string(event.Event.SetID)+":event:"+fmt.Sprint(event.Event.Version) {
		return fmt.Errorf("sealed production Working Set event is invalid")
	}
	diagnostics.events = append(diagnostics.events, event)
	return nil
}

func (diagnostics *productionTraceDiagnostics) finish(
	state *productionTraceState,
) error {
	if err := diagnostics.finishTimings(state); err != nil {
		return err
	}
	return diagnostics.finishWorkingSet(state)
}

func (diagnostics *productionTraceDiagnostics) finishTimings(state *productionTraceState) error {
	if len(diagnostics.timings) != len(state.attempts) ||
		len(state.results)+len(state.abandonments) != len(state.attempts) {
		return fmt.Errorf("sealed production policy timing, attempts, and results differ")
	}
	var elapsed int64
	for callID := range state.attempts {
		timing, exists := diagnostics.timings[callID]
		status, resultExists := state.results[callID]
		_, abandoned := state.abandonments[callID]
		if !exists || resultExists == abandoned || timing.FinishedAt == nil ||
			(resultExists && timing.Status != status) || (abandoned && timing.Status != "abandoned") {
			return fmt.Errorf("sealed production policy timing is not terminal or disagrees with its result")
		}
		elapsed += timing.FinishedAt.Sub(timing.StartedAt).Milliseconds()
	}
	state.metrics.Resources.PolicyWallMilliseconds = elapsed
	state.metrics.Resources.WallMilliseconds = state.trace.Header.SealedAt.
		Sub(state.trace.Header.EpisodeStartedAt).Milliseconds()
	return nil
}

func (diagnostics *productionTraceDiagnostics) finishWorkingSet(state *productionTraceState) error {
	if diagnostics.start == nil || diagnostics.final == nil ||
		diagnostics.start.Snapshot.ID != diagnostics.final.Snapshot.ID ||
		diagnostics.final.Snapshot.Version != state.trace.Header.WorkingSetVersion {
		return fmt.Errorf("sealed production Working Set history lacks exact endpoints")
	}
	set, err := workingset.Restore(diagnostics.start.Snapshot)
	if err != nil {
		return fmt.Errorf("restore sealed production Working Set start: %w", err)
	}
	peak := int64(set.Usage().ResidentBytes)
	for index, supplied := range diagnostics.events {
		if supplied.Event.Version != set.Version()+1 {
			return fmt.Errorf("sealed production Working Set event %d is out of order", index+1)
		}
		command, err := workingset.DecodeCommand(supplied.Event.CommandKind, supplied.Event.Command)
		if err != nil {
			return err
		}
		actual, err := set.Apply(command)
		if err != nil || !reflect.DeepEqual(actual, supplied.Event) {
			return fmt.Errorf("sealed production Working Set event %d replay diverged", index+1)
		}
		resident := int64(set.Usage().ResidentBytes)
		if resident > peak {
			peak = resident
		}
		if supplied.Event.Kind == workingset.EventReacquired {
			state.metrics.Memory.Reacquisitions++
			if supplied.Event.Reacquisition != nil && supplied.Event.Reacquisition.Count > 1 {
				state.metrics.Memory.Thrashes++
			}
		}
	}
	if !reflect.DeepEqual(set.Snapshot(), diagnostics.final.Snapshot) {
		return fmt.Errorf("sealed production Working Set terminal snapshot differs from replay")
	}
	state.metrics.Resources.PeakWorkingSetBytes = peak
	return nil
}
