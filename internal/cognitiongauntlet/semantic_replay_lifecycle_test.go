package cognitiongauntlet

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayLifecycleRetirementIsExactXORWorkerAuthority(t *testing.T) {
	lifecycle, err := cognitionruntime.NewLifecycleCancellationEvidence(
		cognitionruntime.CancellationJobCanceled,
		"Lifecycle canceled the cognition episode.", traceTestDigest("operation"),
	)
	if err != nil {
		t.Fatal(err)
	}
	state := &semanticReplayState{
		trace: productionTrace{Header: queue.CognitionSealedTracePage{
			Seal: queue.CognitionTerminalSeal{LifecycleOperationID: "lifecycle-operation"},
		}},
		cancellation: &lifecycle,
	}
	if err := state.finishLifecycleRetirement(); err == nil {
		t.Fatal("lifecycle cancellation without one retirement was accepted")
	}

	worker, err := cognitionruntime.NewCancellationEvidence(
		cognitionruntime.CancellationPolicyFailure,
		"Worker policy failed.", errors.New("policy failed"),
	)
	if err != nil {
		t.Fatal(err)
	}
	state = &semanticReplayState{
		trace:        productionTrace{Header: queue.CognitionSealedTracePage{}},
		cancellation: &worker,
	}
	if err := state.finishLifecycleRetirement(); err != nil {
		t.Fatalf("worker cancellation without retirement rejected: %v", err)
	}
	state.lifecycleRetirement = &queue.CognitionSealedTraceRecord{}
	if err := state.finishLifecycleRetirement(); err == nil {
		t.Fatal("worker cancellation with lifecycle retirement was accepted")
	}
}
