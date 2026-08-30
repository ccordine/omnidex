package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingRequirementCandidateRecordsExactAcceptedZeroDeltaWithoutInference(t *testing.T) {
	t.Parallel()
	const duplicate = "Display the current status."
	authority := directCodingRequirementGenerationAuthorityFixture(
		t, []string{duplicate}, []string{},
	)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{}, fmt.Errorf("exact zero delta dispatched %q", job.Kind)
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", duplicate, authority,
		directCodingAcceptedRequirementAuthorities(t, []string{duplicate}), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate != duplicate || got.Retain || got.ReboundGenerationAuthority != nil ||
		got.ZeroDelta == nil ||
		got.ZeroDelta.RetainedSet != assemblyline.ApplicationRequirementZeroDeltaAcceptedSet ||
		got.ZeroDelta.RetainedIndex != 0 {
		t.Fatalf("resolved candidate=%+v", got)
	}
}

func TestDirectCodingRequirementCandidateRecordsPostSplitExactZeroDelta(t *testing.T) {
	t.Parallel()
	const accepted = "Display the current status."
	const compound = "Display the current status and show a refresh control."
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
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", compound, authority,
		directCodingAcceptedRequirementAuthorities(t, []string{accepted}), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate != accepted || got.Retain || got.ZeroDelta == nil {
		t.Fatalf("resolved candidate=%+v", got)
	}
	wantCalls := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateSplit,
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", calls, wantCalls)
	}
}

func TestDirectCodingRequirementCandidateRebindsKindWithoutRepeatingTerminalCardinality(
	t *testing.T,
) {
	t.Parallel()
	const compound = "Display a current status and expose a refresh control."
	const atomic = "Display a current status."
	authority := directCodingRequirementGenerationAuthorityFixture(t, []string{}, []string{})
	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			candidate := ""
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
				candidate = atomic
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				candidate = assemblyline.ApplicationRequirementNoDerivedResult
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", compound, authority, []assemblyline.ApplicationIntentCandidateRequirement{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Retain || got.Candidate != atomic {
		t.Fatalf("resolution=%+v", got)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateSplit,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
	}
}

func TestDirectCodingRequirementCandidateRoutesSemanticDuplicateThroughZeroDelta(t *testing.T) {
	t.Parallel()
	const accepted = "Display the current status."
	const candidate = "Show the current status to the user."
	authority := directCodingRequirementGenerationAuthorityFixture(
		t, []string{accepted}, []string{},
	)
	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			value := ""
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateKind:
				value = assemblyline.ApplicationRequirementCandidateTaskLocal
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				value = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
				value = assemblyline.ApplicationRequirementSameRuntimeOutcome
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: value}, nil
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", candidate, authority,
		directCodingAcceptedRequirementAuthorities(t, []string{accepted}), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Retain || got.ZeroDelta == nil ||
		got.ZeroDelta.OutcomeRelation.Relation != assemblyline.ApplicationRequirementSameRuntimeOutcome {
		t.Fatalf("semantic duplicate resolution=%+v", got)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateOutcomeRelation,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
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
		ZeroDeltas:           []assemblyline.ApplicationRequirementCandidateZeroDelta{},
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

func directCodingAcceptedRequirementAuthorities(
	t testing.TB,
	statements []string,
) []assemblyline.ApplicationIntentCandidateRequirement {
	t.Helper()
	result := make([]assemblyline.ApplicationIntentCandidateRequirement, len(statements))
	for index, statement := range statements {
		kind, err := assemblyline.DecodeApplicationRequirementCandidateKindResult(
			assemblyline.ApplicationRequirementCandidateKindInput{Candidate: statement},
			assemblyline.ApplicationRequirementCandidateTaskLocal,
		)
		if err != nil {
			t.Fatal(err)
		}
		cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
			assemblyline.ApplicationRequirementCandidateCardinalityInput{Candidate: statement},
			assemblyline.ApplicationRequirementOneRuntimeOutcome,
		)
		if err != nil {
			t.Fatal(err)
		}
		relation, err := assemblyline.DecodeApplicationRequirementCandidateResultRelationResult(
			assemblyline.ApplicationRequirementCandidateResultRelationInput{
				Candidate: statement, Kind: kind, Cardinality: cardinality,
			},
			assemblyline.ApplicationRequirementNoDerivedResult,
		)
		if err != nil {
			t.Fatal(err)
		}
		result[index] = assemblyline.ApplicationIntentCandidateRequirement{
			Statement: statement, ResultRelation: relation,
		}
	}
	return result
}
