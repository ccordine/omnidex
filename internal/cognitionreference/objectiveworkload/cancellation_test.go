package objectiveworkload_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

func TestCompileCancellationBeforeAndDuringStationIsAuthoritative(t *testing.T) {
	t.Parallel()
	before, cancelBefore := context.WithCancel(context.Background())
	cancelBefore()
	called := 0
	station := stationFunc(func(_ context.Context, _ assemblyline.PortableJob) (assemblyline.PortableResult, error) {
		called++
		return assemblyline.PortableResult{}, nil
	})
	result, err := objectiveworkload.Compile(
		before, "Build a dashboard.", station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if !errors.Is(err, context.Canceled) || called != 0 || result.StationCalls != 0 ||
		result.Authority.Text != "" || result.Compiled {
		t.Fatalf("before cancellation: calls=%d result=%+v err=%v", called, result, err)
	}

	during, cancelDuring := context.WithCancel(context.Background())
	station = stationFunc(func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
		cancelDuring()
		return assemblyline.PortableResult{
			JobID:     job.ID,
			Candidate: `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["dashboard"]}`,
		}, nil
	})
	result, err = objectiveworkload.Compile(
		during, "Build a dashboard.", station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if !errors.Is(err, context.Canceled) || result.StationCalls != 1 || len(result.Gaps) != 1 ||
		result.Gaps[0].Status != objectiveworkload.GapFailed || !result.Gaps[0].ResponseObserved ||
		result.Gaps[0].ResponseSHA256 == "" ||
		result.Gaps[0].OutputSHA256 != "" ||
		result.Compiled || result.Workload.ID != "" {
		t.Fatalf("during cancellation: result=%+v err=%v", result, err)
	}
}

type nthErrorContext struct {
	mu       sync.Mutex
	cancelAt int
	calls    int
	canceled bool
	done     chan struct{}
}

func newNthErrorContext(cancelAt int) *nthErrorContext {
	return &nthErrorContext{cancelAt: cancelAt, done: make(chan struct{})}
}

func (ctx *nthErrorContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *nthErrorContext) Done() <-chan struct{}       { return ctx.done }
func (ctx *nthErrorContext) Value(any) any               { return nil }

func (ctx *nthErrorContext) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.calls++
	if !ctx.canceled && ctx.calls >= ctx.cancelAt {
		ctx.canceled = true
		close(ctx.done)
	}
	if ctx.canceled {
		return context.Canceled
	}
	return nil
}

func TestCompileChecksCancellationAfterFinalGraphConstruction(t *testing.T) {
	t.Parallel()
	ctx := newNthErrorContext(9)
	station := stationFunc(func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
		var input assemblyline.RequirementPartitionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return assemblyline.PortableResult{}, err
		}
		quotes := []string{}
		if input.Mode == assemblyline.RequirementExtractFeatures && input.SourceText == "Build a dashboard." {
			quotes = []string{"dashboard"}
		} else if input.Mode == assemblyline.RequirementSplitFeature {
			quotes = []string{input.SourceText}
		}
		raw, err := json.Marshal(assemblyline.RequirementPartitionDecision{
			Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: quotes,
		})
		return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
	})
	result, err := objectiveworkload.Compile(
		ctx, "Build a dashboard.", station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if !errors.Is(err, context.Canceled) || result.Compiled || result.Workload.ID != "" ||
		result.StationCalls != 3 || len(result.Gaps) != 3 {
		t.Fatalf("result=%+v err=%v err calls=%d", result, err, ctx.calls)
	}
	for _, gap := range result.Gaps {
		if gap.Status != objectiveworkload.GapResolved || gap.FinalWorkloadID != "" {
			t.Fatalf("final-boundary cancellation gap=%+v", gap)
		}
	}
}

type cancelOperations struct {
	cancelAt string
	cancel   context.CancelFunc
}

func (operations cancelOperations) Materialize(
	_ context.Context,
	item objectiveworkload.WorkItem,
) (objectiveworkload.ArtifactValue, error) {
	if operations.cancelAt == "materialize" {
		operations.cancel()
	}
	return objectiveworkload.ArtifactValue{
		Kind:    objectiveworkload.ArtifactRequirementOutput,
		Content: []byte("accepted\x00" + item.Requirement.SourceQuote),
	}, nil
}

func (operations cancelOperations) Verify(
	_ context.Context,
	_ objectiveworkload.WorkItem,
	_ objectiveworkload.Artifact,
) error {
	if operations.cancelAt == "verify" {
		operations.cancel()
	}
	return nil
}

func TestRunCancellationBeforeAndDuringOperationsIsAuthoritative(t *testing.T) {
	t.Parallel()
	workload := compileOne(t)
	before, cancelBefore := context.WithCancel(context.Background())
	cancelBefore()
	result, err := objectiveworkload.Run(
		before, workload, &scriptedOperations{},
		objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
	)
	if !errors.Is(err, context.Canceled) || result.WorkloadID != "" || result.Complete {
		t.Fatalf("before cancellation: result=%+v err=%v", result, err)
	}

	for _, at := range []string{"materialize", "verify"} {
		at := at
		t.Run(at, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			result, err := objectiveworkload.Run(
				ctx, workload, cancelOperations{cancelAt: at, cancel: cancel},
				objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
			)
			if !errors.Is(err, context.Canceled) || result.WorkloadID != workload.ID || result.Complete {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if at == "materialize" && (len(result.Trace) != 0 || len(result.Artifacts) != 0 || result.DeterministicOperationCalls != 1) {
				t.Fatalf("materialize cancellation partial=%+v", result)
			}
			if at == "verify" && (len(result.Trace) != 1 || len(result.Artifacts) != 1 || result.DeterministicOperationCalls != 2) {
				t.Fatalf("verify cancellation partial=%+v", result)
			}
		})
	}
}

func TestRunChecksCancellationAtTerminalCompletionBoundary(t *testing.T) {
	t.Parallel()
	workload := compileOne(t)
	ctx := newNthErrorContext(21)
	result, err := objectiveworkload.Run(
		ctx, workload, constantArtifactOperations{},
		objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
	)
	if !errors.Is(err, context.Canceled) || result.Complete || result.WorkloadID != workload.ID ||
		len(result.Trace) != 4 || len(result.Artifacts) != 1 || result.DeterministicOperationCalls != 2 {
		t.Fatalf("terminal cancellation result=%+v err=%v err calls=%d", result, err, ctx.calls)
	}
}
