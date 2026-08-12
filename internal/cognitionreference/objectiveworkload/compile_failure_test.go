package objectiveworkload_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

type stationFunc func(context.Context, assemblyline.PortableJob) (assemblyline.PortableResult, error)

func (function stationFunc) Generate(
	ctx context.Context,
	job assemblyline.PortableJob,
) (assemblyline.PortableResult, error) {
	return function(ctx, job)
}

func TestCompileReturnsHonestPartialStateOnStationFailure(t *testing.T) {
	t.Parallel()
	authority := "  Make one dashboard.  "
	stationErr := errors.New("semantic endpoint stopped")
	station := &scriptedPartitionStation{
		steps: newPartitionScript(authority, "dashboard"), errAt: 2, err: stationErr,
	}
	result, err := objectiveworkload.Compile(
		context.Background(), authority, station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if !errors.Is(err, stationErr) {
		t.Fatalf("error=%v", err)
	}
	if result.Compiled || result.Workload.ID != "" || len(result.Workload.Objectives) != 0 {
		t.Fatalf("failure returned plausible workload: %+v", result)
	}
	if result.StationCalls != 2 || len(result.Gaps) != 2 {
		t.Fatalf("partial calls=%d gaps=%d", result.StationCalls, len(result.Gaps))
	}
	if result.Gaps[0].Status != objectiveworkload.GapResolved ||
		result.Gaps[1].Status != objectiveworkload.GapFailed {
		t.Fatalf("gap statuses=%+v", result.Gaps)
	}
	if result.Gaps[0].OutputSHA256 == "" || result.Gaps[1].OutputSHA256 != "" {
		t.Fatalf("accepted/failed output authority=%+v", result.Gaps)
	}
	if result.Gaps[0].ResponseSHA256 == "" || result.Gaps[1].ResponseSHA256 != "" ||
		!result.Gaps[0].ResponseObserved || result.Gaps[1].ResponseObserved {
		t.Fatalf("received response authority=%+v", result.Gaps)
	}
}

func TestProviderErrorRecordsOnlyActuallyReturnedResponse(t *testing.T) {
	t.Parallel()
	providerErr := errors.New("provider stopped")
	tests := []struct {
		name     string
		returned assemblyline.PortableResult
		observed bool
	}{
		{name: "no response", returned: assemblyline.PortableResult{}, observed: false},
		{
			name: "response with error",
			returned: assemblyline.PortableResult{
				JobID: "wrong", Candidate: `{"schema":"partial"}`,
			},
			observed: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			station := stationFunc(func(_ context.Context, _ assemblyline.PortableJob) (assemblyline.PortableResult, error) {
				return test.returned, providerErr
			})
			result, err := objectiveworkload.Compile(
				context.Background(), "Build a dashboard.", station,
				objectiveworkload.CompileLimits{MaxStationCalls: 8},
			)
			if !errors.Is(err, providerErr) || result.Compiled || result.StationCalls != 1 || len(result.Gaps) != 1 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			gap := result.Gaps[0]
			if gap.ResponseObserved != test.observed || (test.observed && gap.ResponseSHA256 == "") ||
				(!test.observed && gap.ResponseSHA256 != "") || gap.OutputSHA256 != "" {
				t.Fatalf("gap=%+v", gap)
			}
		})
	}
}

func TestEmptySuccessfulStationReturnIsObservedButRejected(t *testing.T) {
	t.Parallel()
	station := stationFunc(func(_ context.Context, _ assemblyline.PortableJob) (assemblyline.PortableResult, error) {
		return assemblyline.PortableResult{}, nil
	})
	result, err := objectiveworkload.Compile(
		context.Background(), "Build a dashboard.", station,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if err == nil || result.Compiled || result.StationCalls != 1 || len(result.Gaps) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	gap := result.Gaps[0]
	if !gap.ResponseObserved || !gap.ResponseWithinBounds || gap.ResponseSHA256 == "" ||
		gap.ResponseJobIDBytes != 0 || gap.ResponseCandidateBytes != 0 || gap.OutputSHA256 != "" {
		t.Fatalf("empty returned response receipt=%+v", gap)
	}
}

func TestCompileRejectsInvalidPortableCandidatesWithoutFallback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		candidate func(assemblyline.PortableJob) assemblyline.PortableResult
	}{
		{
			name: "wrong job",
			candidate: func(job assemblyline.PortableJob) assemblyline.PortableResult {
				return assemblyline.PortableResult{JobID: strings.Repeat("0", 64), Candidate: `{}`}
			},
		},
		{
			name: "unknown field",
			candidate: func(job assemblyline.PortableJob) assemblyline.PortableResult {
				return assemblyline.PortableResult{JobID: job.ID, Candidate: `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["dashboard"],"plan":[]}`}
			},
		},
		{
			name: "inexact alias",
			candidate: func(job assemblyline.PortableJob) assemblyline.PortableResult {
				return assemblyline.PortableResult{JobID: job.ID, Candidate: `{"Schema":"omnidex.requirement-partition.v1","feature_quotes":["dashboard"]}`}
			},
		},
		{
			name: "duplicate schema",
			candidate: func(job assemblyline.PortableJob) assemblyline.PortableResult {
				return assemblyline.PortableResult{JobID: job.ID, Candidate: `{"schema":"omnidex.requirement-partition.v1","schema":"omnidex.requirement-partition.v1","feature_quotes":["dashboard"]}`}
			},
		},
		{
			name: "duplicate feature quotes",
			candidate: func(job assemblyline.PortableJob) assemblyline.PortableResult {
				return assemblyline.PortableResult{JobID: job.ID, Candidate: `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["dashboard"],"feature_quotes":["dashboard"]}`}
			},
		},
		{
			name: "trailing value",
			candidate: func(job assemblyline.PortableJob) assemblyline.PortableResult {
				return assemblyline.PortableResult{JobID: job.ID, Candidate: `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["dashboard"]}{}`}
			},
		},
		{
			name: "paraphrase",
			candidate: func(job assemblyline.PortableJob) assemblyline.PortableResult {
				return assemblyline.PortableResult{JobID: job.ID, Candidate: `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["analytics screen"]}`}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			station := stationFunc(func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
				calls++
				return test.candidate(job), nil
			})
			result, err := objectiveworkload.Compile(
				context.Background(), "Build a dashboard.", station,
				objectiveworkload.CompileLimits{MaxStationCalls: 4},
			)
			if err == nil {
				t.Fatal("invalid candidate unexpectedly compiled")
			}
			if calls != 1 || result.StationCalls != 1 || result.Compiled || result.Workload.ID != "" {
				t.Fatalf("calls=%d result=%+v", calls, result)
			}
			if len(result.Gaps) != 1 || result.Gaps[0].ResponseSHA256 == "" ||
				result.Gaps[0].ResponseCandidateSHA256 == "" || result.Gaps[0].OutputSHA256 != "" {
				t.Fatalf("failed response authority=%+v", result.Gaps)
			}
			if result.Gaps[0].ResponseJobIDBytes == 0 || result.Gaps[0].ResponseCandidateBytes == 0 {
				t.Fatalf("failed response lacks exact returned identity/size: %+v", result.Gaps[0])
			}
			if test.name == "wrong job" && result.Gaps[0].ResponseJobIDMatches {
				t.Fatalf("wrong returned job ID was recorded as matching: %+v", result.Gaps[0])
			}
		})
	}
}
