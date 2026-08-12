package objectiveworkload_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

func TestCompileRejectsInvalidExactAuthorityBeforeDispatch(t *testing.T) {
	t.Parallel()
	invalid := []string{"", " \n\t ", "Build\x00dashboard", string([]byte{0xff}), strings.Repeat("x", 9*1024)}
	for _, authority := range invalid {
		authority := authority
		t.Run("invalid", func(t *testing.T) {
			t.Parallel()
			calls := 0
			station := stationFunc(func(_ context.Context, _ assemblyline.PortableJob) (assemblyline.PortableResult, error) {
				calls++
				return assemblyline.PortableResult{}, nil
			})
			result, err := objectiveworkload.Compile(
				context.Background(), authority, station,
				objectiveworkload.CompileLimits{MaxStationCalls: 8},
			)
			if err == nil || calls != 0 || result.StationCalls != 0 || result.Authority.Text != "" || result.Compiled {
				t.Fatalf("calls=%d result=%+v err=%v", calls, result, err)
			}
		})
	}
}

func TestCompileRejectsStationMutationOfFrozenJob(t *testing.T) {
	t.Parallel()
	station := stationFunc(func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
		jobID := job.ID
		job.Payload[0] ^= 0xff
		return assemblyline.PortableResult{
			JobID:     jobID,
			Candidate: `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["dashboard"]}`,
		}, nil
	})
	result, err := objectiveworkload.Compile(
		context.Background(), "Build a dashboard.", station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if err == nil || result.StationCalls != 1 || len(result.Gaps) != 1 ||
		result.Gaps[0].Status != objectiveworkload.GapFailed || !result.Gaps[0].ResponseObserved ||
		result.Gaps[0].ResponseSHA256 == "" ||
		result.Gaps[0].OutputSHA256 != "" ||
		result.Compiled || result.Workload.ID != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCompileRejectsOverlappingDuplicateGrounding(t *testing.T) {
	t.Parallel()
	station := stationFunc(func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
		var input assemblyline.RequirementPartitionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return assemblyline.PortableResult{}, err
		}
		quotes := []string{}
		if input.Mode == assemblyline.RequirementExtractFeatures && input.SourceText == "aaa" {
			quotes = []string{"aa"}
		} else if input.Mode == assemblyline.RequirementSplitFeature {
			quotes = []string{input.SourceText}
		}
		raw, err := json.Marshal(assemblyline.RequirementPartitionDecision{
			Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: quotes,
		})
		return assemblyline.PortableResult{
			JobID: job.ID, Candidate: string(raw),
		}, err
	})
	result, err := objectiveworkload.Compile(
		context.Background(), "aaa", station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if err == nil || result.Compiled || result.Workload.ID != "" || result.StationCalls != 3 ||
		len(result.Gaps) != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestTypedNilStationAndOperationsFailWithoutPanic(t *testing.T) {
	t.Parallel()
	var station stationFunc
	compiled, err := objectiveworkload.Compile(
		context.Background(), "Build a dashboard.", station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if err == nil || compiled.StationCalls != 0 || compiled.Compiled {
		t.Fatalf("typed nil station result=%+v err=%v", compiled, err)
	}

	workload := compileOne(t)
	var operations nilMapOperations
	run, err := objectiveworkload.Run(
		context.Background(), workload, operations,
		objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
	)
	if err == nil || run.WorkloadID != "" || run.DeterministicOperationCalls != 0 || run.Complete {
		t.Fatalf("typed nil operations result=%+v err=%v", run, err)
	}
}

type nilMapOperations map[string]bool

func (nilMapOperations) Materialize(
	context.Context,
	objectiveworkload.WorkItem,
) (objectiveworkload.ArtifactValue, error) {
	panic("typed nil operations must be rejected before dispatch")
}

func (nilMapOperations) Verify(
	context.Context,
	objectiveworkload.WorkItem,
	objectiveworkload.Artifact,
) error {
	panic("typed nil operations must be rejected before dispatch")
}

func TestUnboundedInvalidStationReturnFailsBeforeHashingOrRetention(t *testing.T) {
	t.Parallel()
	tests := []assemblyline.PortableResult{
		{JobID: strings.Repeat("x", 129), Candidate: `{}`},
		{JobID: strings.Repeat("0", 64), Candidate: strings.Repeat("x", 13*1024)},
	}
	for _, returned := range tests {
		returned := returned
		t.Run("oversized", func(t *testing.T) {
			t.Parallel()
			station := stationFunc(func(_ context.Context, _ assemblyline.PortableJob) (assemblyline.PortableResult, error) {
				return returned, nil
			})
			result, err := objectiveworkload.Compile(
				context.Background(), "Build a dashboard.", station,
				objectiveworkload.CompileLimits{MaxStationCalls: 8},
			)
			if err == nil || result.Compiled || result.Workload.ID != "" || result.StationCalls != 1 ||
				len(result.Gaps) != 1 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			gap := result.Gaps[0]
			if !gap.ResponseObserved || gap.ResponseWithinBounds || gap.ResponseSHA256 != "" ||
				gap.ResponseCandidateSHA256 != "" || gap.ResponseJobIDSHA256 != "" || gap.OutputSHA256 != "" {
				t.Fatalf("unbounded response was hashed or retained as accepted: %+v", gap)
			}
		})
	}
}
