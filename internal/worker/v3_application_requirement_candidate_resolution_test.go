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

func TestDirectCodingRequirementCandidateRepairsAcceptedDuplicateOnce(t *testing.T) {
	t.Parallel()
	const duplicate = "Display the current status."
	const replacement = "Show a refresh control."
	authority := directCodingRequirementGenerationAuthorityFixture(
		t, []string{duplicate}, []string{},
	)
	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateDuplicateReplacement:
				var input assemblyline.ApplicationRequirementCandidateDuplicateReplacementInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if !reflect.DeepEqual(input.GenerationAuthority, authority) ||
					input.CurrentCandidate != duplicate ||
					input.Duplicate.Set != assemblyline.ApplicationRequirementDuplicateAcceptedRequirement ||
					input.Duplicate.Index != 0 ||
					input.Defect != assemblyline.ApplicationRequirementDuplicateCandidateDefect {
					return assemblyline.PortableResult{}, fmt.Errorf("replacement received unbound duplicate authority: %+v", input)
				}
				candidate = replacement
			case assemblyline.WorkApplicationRequirementCandidateKind:
				candidate = assemblyline.ApplicationRequirementCandidateTaskLocal
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", duplicate, authority, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate != replacement || !got.Retain || got.ReboundGenerationAuthority != nil {
		t.Fatalf("resolved candidate=%+v", got)
	}
	wantCalls := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateDuplicateReplacement,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", calls, wantCalls)
	}
}

func TestDirectCodingRequirementCandidateRepairsPostSplitDuplicateThenReclassifies(t *testing.T) {
	t.Parallel()
	const accepted = "Display the current status."
	const compound = "Display the current status and show a refresh control."
	const replacement = "Show a refresh control."
	authority := directCodingRequirementGenerationAuthorityFixture(
		t, []string{accepted}, []string{},
	)
	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateKind:
				candidate = assemblyline.ApplicationRequirementCandidateTaskLocal
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
				candidate = accepted
			case assemblyline.WorkApplicationRequirementCandidateDuplicateReplacement:
				var input assemblyline.ApplicationRequirementCandidateDuplicateReplacementInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.CurrentCandidate != accepted ||
					input.Duplicate.Set != assemblyline.ApplicationRequirementDuplicateAcceptedRequirement ||
					input.Duplicate.Index != 0 {
					return assemblyline.PortableResult{}, fmt.Errorf("post-split replacement received wrong duplicate: %+v", input)
				}
				candidate = replacement
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", compound, authority, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate != replacement || !got.Retain || got.ReboundGenerationAuthority != nil {
		t.Fatalf("resolved candidate=%+v", got)
	}
	wantCalls := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateSplit,
		assemblyline.WorkApplicationRequirementCandidateDuplicateReplacement,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", calls, wantCalls)
	}
}

func TestDirectCodingRequirementDuplicateReplacementRejectsAnyRetainedDuplicate(t *testing.T) {
	t.Parallel()
	const accepted = "Display the current status."
	const excluded = "Use a single source file."
	authority := directCodingRequirementGenerationAuthorityFixture(
		t, []string{accepted}, []string{excluded},
	)
	for name, replacement := range map[string]string{
		"byte identical":               accepted,
		"different retained duplicate": excluded,
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			var finalizedValidation error
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					if job.Kind != assemblyline.WorkApplicationRequirementCandidateDuplicateReplacement {
						return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: replacement}, nil
				},
				Finalize: func(
					_ assemblyline.PortableJob,
					_ assemblyline.PortableResult,
					validationErr error,
				) error {
					finalizedValidation = validationErr
					return nil
				},
			}
			_, err := resolveDirectCodingApplicationRequirementCandidate(
				runtime, "intent-model", accepted, authority, nil,
			)
			if err == nil || calls != 1 || finalizedValidation == nil ||
				!strings.Contains(err.Error(), "duplicate") {
				t.Fatalf(
					"replacement=%q calls=%d finalized=%v error=%v",
					replacement, calls, finalizedValidation, err,
				)
			}
		})
	}
}

func directCodingRequirementGenerationAuthorityFixture(
	t testing.TB,
	accepted []string,
	excluded []string,
) assemblyline.ApplicationRequirementCandidateInput {
	t.Helper()
	request := "Create a browser status board with a current status and refresh control."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	coverageInput := assemblyline.ApplicationRequirementCoverageInput{
		UserRequest: request, Context: applicationContext,
		AcceptedRequirements: append([]string{}, accepted...),
		ExcludedCandidates:   append([]string{}, excluded...),
	}
	coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
		coverageInput, assemblyline.ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirementCandidateInput{
		Authority: coverageInput, Coverage: coverage,
	}
}
