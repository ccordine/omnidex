package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingRequirementCandidateRefinementSplitsBeforeRetention(t *testing.T) {
	t.Parallel()
	const compound = "Provide interactive drum pads and a playable keyboard."
	const atomic = "Provide interactive drum pads."
	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				var input assemblyline.ApplicationRequirementCandidateCardinalityInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == compound {
					candidate = assemblyline.ApplicationRequirementMultipleRuntimeOutcomes
				} else if input.Candidate == atomic {
					candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
				} else {
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected cardinality candidate %q", input.Candidate)
				}
			case assemblyline.WorkApplicationRequirementCandidateSplit:
				var input assemblyline.ApplicationRequirementCandidateSplitInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate != compound ||
					input.Cardinality.Relation != assemblyline.ApplicationRequirementMultipleRuntimeOutcomes {
					return assemblyline.PortableResult{}, fmt.Errorf("split received unbound authority")
				}
				candidate = atomic
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := refineDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", compound,
		assemblyline.ApplicationRequirementCoverageInput{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate != atomic ||
		got.Cardinality.Relation != assemblyline.ApplicationRequirementOneRuntimeOutcome {
		t.Fatalf("refined=%+v", got)
	}
	wantCalls := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateSplit,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", calls, wantCalls)
	}
}

func TestDirectCodingRequirementCandidateRefinementFailsAtSplitBound(t *testing.T) {
	t.Parallel()
	cardinalityCalls, splitCalls := 0, 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				cardinalityCalls++
				candidate = assemblyline.ApplicationRequirementMultipleRuntimeOutcomes
			case assemblyline.WorkApplicationRequirementCandidateSplit:
				splitCalls++
				candidate = fmt.Sprintf("Retain outcome %d and another outcome.", splitCalls)
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	_, err := refineDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", "Retain a first outcome and a second outcome.",
		assemblyline.ApplicationRequirementCoverageInput{}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "split bound") {
		t.Fatalf("split-bound error=%v", err)
	}
	if splitCalls != assemblyline.MaxApplicationRequirementCandidateSplitDepth ||
		cardinalityCalls != splitCalls+1 {
		t.Fatalf("cardinality calls=%d split calls=%d", cardinalityCalls, splitCalls)
	}
}

func TestDirectCodingRequirementCandidateRefinementCorrectsOneExactUnchangedSplit(t *testing.T) {
	t.Parallel()
	const compound = "Provide interactive drum pads and a playable keyboard."
	const atomic = "Provide interactive drum pads."
	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				var input assemblyline.ApplicationRequirementCandidateCardinalityInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == compound {
					candidate = assemblyline.ApplicationRequirementMultipleRuntimeOutcomes
				} else {
					candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
				}
			case assemblyline.WorkApplicationRequirementCandidateSplit:
				candidate = compound
			case assemblyline.WorkApplicationRequirementCandidateSplitCorrection:
				var input assemblyline.ApplicationRequirementCandidateSplitCorrectionInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.CurrentCandidate != compound ||
					input.Defect != assemblyline.ApplicationRequirementUnchangedSplitDefect {
					return assemblyline.PortableResult{}, fmt.Errorf("correction received ungrounded authority")
				}
				candidate = atomic
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := refineDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", compound,
		assemblyline.ApplicationRequirementCoverageInput{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate != atomic ||
		got.Cardinality.Relation != assemblyline.ApplicationRequirementOneRuntimeOutcome {
		t.Fatalf("refined=%+v", got)
	}
	wantCalls := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateSplit,
		assemblyline.WorkApplicationRequirementCandidateSplitCorrection,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", calls, wantCalls)
	}
}
