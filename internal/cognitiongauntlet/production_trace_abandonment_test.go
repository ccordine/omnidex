package cognitiongauntlet

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func TestProductionDiagnosticsAdmitExactAbandonedCallWithoutModelCall(t *testing.T) {
	started := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	finished := started.Add(time.Second)
	state := newProductionTraceState(
		productionTrace{Header: queue.CognitionSealedTracePage{
			EpisodeStartedAt: started, SealedAt: finished,
		}}, RecoveryMetrics{}, cognitionpolicy.AttestedBrain{},
	)
	state.attempts["call-1"] = cognitionpolicy.CallAttempt{}
	state.abandonments["call-1"] = cognitionruntime.PolicyCallAbandonment{}
	state.diagnostics.timings["call-1"] = queue.CognitionTracePolicyTiming{
		Schema: queue.CognitionTracePolicyTimingSchemaV1, CallID: "call-1",
		Status: "abandoned", StartedAt: started, FinishedAt: &finished,
	}
	if err := state.diagnostics.finishTimings(state); err != nil {
		t.Fatal(err)
	}
	if state.metrics.Resources.ModelCalls != 0 {
		t.Fatalf("abandoned call counted as model call: %+v", state.metrics.Resources)
	}
}
