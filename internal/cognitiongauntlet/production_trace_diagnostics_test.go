package cognitiongauntlet

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestProductionDiagnosticsDeriveDurableTimingAndWorkingSetMetrics(t *testing.T) {
	startedAt := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	set, events := diagnosticWorkingSet(t)
	header := queue.CognitionSealedTracePage{
		EpisodeStartedAt: startedAt, SealedAt: startedAt.Add(250 * time.Millisecond),
		WorkingSetVersion: set.Version(),
	}
	diagnostics := newProductionTraceDiagnostics()
	startSet, err := workingset.New(set.Owner(), set.Budget())
	if err != nil {
		t.Fatal(err)
	}
	acceptDiagnosticPayload(t, &diagnostics, header, "working_set_snapshot",
		string(set.ID())+":episode-start", queue.CognitionTraceWorkingSetSnapshot{
			Schema: queue.CognitionTraceWorkingSetSnapshotSchemaV1, Point: "episode_start",
			CapturedAt: startedAt, Snapshot: startSet.Snapshot(),
		})
	for index, event := range events {
		acceptDiagnosticPayload(t, &diagnostics, header, "working_set_event",
			string(set.ID())+":event:"+itoa(event.Version), queue.CognitionTraceWorkingSetEvent{
				Schema:    queue.CognitionTraceWorkingSetEventSchemaV1,
				CreatedAt: startedAt.Add(time.Duration(index+1) * time.Millisecond), Event: event,
			})
	}
	acceptDiagnosticPayload(t, &diagnostics, header, "working_set_snapshot",
		string(set.ID())+":terminal", queue.CognitionTraceWorkingSetSnapshot{
			Schema: queue.CognitionTraceWorkingSetSnapshotSchemaV1, Point: "terminal",
			CapturedAt: startedAt.Add(4 * time.Millisecond), Snapshot: set.Snapshot(),
		})
	finishedAt := startedAt.Add(25 * time.Millisecond)
	acceptDiagnosticPayload(t, &diagnostics, header, "policy_timing", "call-1:timing",
		queue.CognitionTracePolicyTiming{
			Schema: queue.CognitionTracePolicyTimingSchemaV1, CallID: "call-1",
			Status: cognitionpolicy.CallResultAccepted, StartedAt: startedAt, FinishedAt: &finishedAt,
		})

	state := newProductionTraceState(
		productionTrace{Header: header}, RecoveryMetrics{}, cognitionpolicy.AttestedBrain{},
	)
	state.diagnostics = diagnostics
	state.attempts["call-1"] = cognitionpolicy.CallAttempt{}
	state.results["call-1"] = cognitionpolicy.CallResultAccepted
	if err := state.diagnostics.finish(state); err != nil {
		t.Fatal(err)
	}
	if state.metrics.Resources.PolicyWallMilliseconds != 25 ||
		state.metrics.Resources.WallMilliseconds != 250 ||
		state.metrics.Resources.PeakWorkingSetBytes != 17 ||
		state.metrics.Memory.Reacquisitions != 1 || state.metrics.Memory.Thrashes != 0 {
		t.Fatalf("derived diagnostics = %+v %+v", state.metrics.Resources, state.metrics.Memory)
	}
}

func TestProductionDiagnosticsRejectIncompleteWorkingSetReplay(t *testing.T) {
	startedAt := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	set, _ := diagnosticWorkingSet(t)
	header := queue.CognitionSealedTracePage{
		EpisodeStartedAt: startedAt, SealedAt: startedAt.Add(time.Second),
		WorkingSetVersion: set.Version(),
	}
	diagnostics := newProductionTraceDiagnostics()
	startSet, err := workingset.New(set.Owner(), set.Budget())
	if err != nil {
		t.Fatal(err)
	}
	acceptDiagnosticPayload(t, &diagnostics, header, "working_set_snapshot",
		string(set.ID())+":episode-start", queue.CognitionTraceWorkingSetSnapshot{
			Schema: queue.CognitionTraceWorkingSetSnapshotSchemaV1, Point: "episode_start",
			CapturedAt: startedAt, Snapshot: startSet.Snapshot(),
		})
	acceptDiagnosticPayload(t, &diagnostics, header, "working_set_snapshot",
		string(set.ID())+":terminal", queue.CognitionTraceWorkingSetSnapshot{
			Schema: queue.CognitionTraceWorkingSetSnapshotSchemaV1, Point: "terminal",
			CapturedAt: startedAt.Add(time.Millisecond), Snapshot: set.Snapshot(),
		})
	state := newProductionTraceState(
		productionTrace{Header: header}, RecoveryMetrics{}, cognitionpolicy.AttestedBrain{},
	)
	state.diagnostics = diagnostics
	if err := state.diagnostics.finishWorkingSet(state); err == nil {
		t.Fatal("incomplete sealed Working Set event stream was accepted")
	}
}

func diagnosticWorkingSet(t *testing.T) (*workingset.Set, []workingset.Event) {
	t.Helper()
	ledgerID, err := taskstate.NewLedgerID(taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: 91, RunID: "01234567-89ab-cdef-0123-456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	set, err := workingset.New(workingset.Owner{LedgerID: ledgerID, JobID: 91, Generation: 1},
		workingset.Budget{MaxItems: 2, MaxBytes: 32, MaxPinnedItems: 1, MaxPinnedBytes: 16})
	if err != nil {
		t.Fatal(err)
	}
	scope := workingset.Scope{Kind: workingset.ScopeTask, ID: "task-diagnostic"}
	ref := taskstate.Ref{
		URI: "evidence://episode/observation", Version: "1", Hash: traceTestDigest("evidence"),
		Relation: taskstate.RefEvidence,
	}
	acquireID, _ := workingset.NewCommandID("production-diagnostic", "acquire")
	releaseID, _ := workingset.NewCommandID("production-diagnostic", "release")
	reacquireID, _ := workingset.NewCommandID("production-diagnostic", "reacquire")
	commands := []workingset.Command{
		workingset.AcquireCommand{CommandID: acquireID, Actor: taskstate.AuthorityCode,
			Request: workingset.AcquireRequest{
				ID: "diagnostic-item", Ref: ref, Role: workingset.RoleEvidence,
				Retention: workingset.RetentionTask, Scope: scope, Priority: 50, ByteCost: 17,
				Acquisition: workingset.Acquisition{Provider: workingset.ProviderEvidence,
					OperationID: "diagnostic-acquisition", Reason: "Required by the current diagnostic."},
			}},
		workingset.ReleaseCommand{CommandID: releaseID, ExpectedVersion: 1,
			Actor: taskstate.AuthorityCode, ItemID: "diagnostic-item", Scope: scope,
			Reason: "The diagnostic released this evidence."},
		workingset.ReacquireCommand{CommandID: reacquireID, ExpectedVersion: 2,
			Actor: taskstate.AuthorityCode, Request: workingset.ReacquireRequest{
				ItemID: "diagnostic-item", Ref: ref, Scope: scope, Retention: workingset.RetentionTask,
				Reason: "The diagnostic requires the exact evidence again."}},
	}
	events := make([]workingset.Event, len(commands))
	for index, command := range commands {
		events[index], err = set.Apply(command)
		if err != nil {
			t.Fatalf("apply diagnostic command %d: %v", index, err)
		}
	}
	return set, events
}

func acceptDiagnosticPayload(
	t *testing.T,
	diagnostics *productionTraceDiagnostics,
	header queue.CognitionSealedTracePage,
	kind, id string,
	payload any,
) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := diagnostics.accept(queue.CognitionSealedTraceRecord{
		Kind: kind, ID: id, Payload: raw,
	}, header); err != nil {
		t.Fatal(err)
	}
}

func itoa(value uint64) string {
	return fmt.Sprint(value)
}
