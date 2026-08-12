package objectiveworkload_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

func TestCompileAndRunBoundsFailWithoutFallback(t *testing.T) {
	t.Parallel()
	authority := "Build a dashboard."
	for _, limit := range []int{0, 129} {
		station := &scriptedPartitionStation{steps: newPartitionScript(authority, "dashboard")}
		result, err := objectiveworkload.Compile(
			context.Background(), authority, station,
			objectiveworkload.CompileLimits{MaxStationCalls: limit},
		)
		if err == nil || result.StationCalls != 0 || station.calls != 0 || result.Compiled {
			t.Fatalf("compile limit=%d result=%+v station calls=%d err=%v", limit, result, station.calls, err)
		}
	}

	station := &scriptedPartitionStation{steps: newPartitionScript(authority, "dashboard")}
	result, err := objectiveworkload.Compile(
		context.Background(), authority, station,
		objectiveworkload.CompileLimits{MaxStationCalls: 1},
	)
	if !errors.Is(err, objectiveworkload.ErrCompileBound) || result.StationCalls != 1 ||
		len(result.Gaps) != 1 || result.Gaps[0].Status != objectiveworkload.GapResolved ||
		result.Compiled || result.Workload.ID != "" {
		t.Fatalf("compile call bound result=%+v err=%v", result, err)
	}

	workload := compileOne(t)
	operations := &scriptedOperations{}
	run, err := objectiveworkload.Run(
		context.Background(), workload, operations,
		objectiveworkload.RunLimits{MaxTransitions: 1, MaxDepth: 8},
	)
	if !errors.Is(err, objectiveworkload.ErrRunBound) || run.WorkloadID != workload.ID ||
		len(run.Trace) != 1 || len(run.Artifacts) != 1 || run.Complete ||
		run.DeterministicOperationCalls != 1 {
		t.Fatalf("transition bound result=%+v err=%v", run, err)
	}

	operations = &scriptedOperations{}
	run, err = objectiveworkload.Run(
		context.Background(), workload, operations,
		objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 2},
	)
	if !errors.Is(err, objectiveworkload.ErrRunBound) || run.WorkloadID != workload.ID ||
		len(run.Trace) != 0 || run.DeterministicOperationCalls != 0 || run.Complete {
		t.Fatalf("depth bound result=%+v err=%v", run, err)
	}
}

type oversizedArtifactOperations struct{}

func (oversizedArtifactOperations) Materialize(
	context.Context,
	objectiveworkload.WorkItem,
) (objectiveworkload.ArtifactValue, error) {
	return objectiveworkload.ArtifactValue{
		Kind: objectiveworkload.ArtifactRequirementOutput, Content: []byte(strings.Repeat("x", 4*1024*1024)),
	}, nil
}

func (oversizedArtifactOperations) Verify(
	context.Context,
	objectiveworkload.WorkItem,
	objectiveworkload.Artifact,
) error {
	return nil
}

func TestOpaqueArtifactBoundFailsBeforeVerification(t *testing.T) {
	t.Parallel()
	workload := compileOne(t)
	result, err := objectiveworkload.Run(
		context.Background(), workload, oversizedArtifactOperations{},
		objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
	)
	if !errors.Is(err, objectiveworkload.ErrArtifact) || result.Complete ||
		result.DeterministicOperationCalls != 1 || len(result.Trace) != 0 || len(result.Artifacts) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestPartitionOwnValidationRejectsEmptyOverlapAndNoProgress(t *testing.T) {
	t.Parallel()
	tests := []assemblyline.RequirementPartitionDecision{
		{Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: []string{}},
		{Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: []string{"dashboard", "dashboard"}},
	}
	for _, decision := range tests {
		calls := 0
		station := stationFunc(func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			calls++
			raw := `{"schema":"omnidex.requirement-partition.v1","feature_quotes":[]}`
			if len(decision.FeatureQuotes) > 0 {
				raw = `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["dashboard","dashboard"]}`
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
		})
		result, err := objectiveworkload.Compile(
			context.Background(), "Build a dashboard.", station,
			objectiveworkload.CompileLimits{MaxStationCalls: 8},
		)
		if err == nil || calls == 0 || result.Compiled || result.Workload.ID != "" {
			t.Fatalf("decision=%+v calls=%d result=%+v err=%v", decision, calls, result, err)
		}
	}
}
