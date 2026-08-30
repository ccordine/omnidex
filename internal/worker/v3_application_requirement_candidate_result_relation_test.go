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

func TestDirectCodingRequirementCorrectsMissingResultRelationBeforeRetention(t *testing.T) {
	t.Parallel()
	const request = "Build a browser distance converter that converts miles to kilometers using 1 mile = 1.609344 kilometers."
	const vague = "Accept a distance and display an accurate converted result."
	const corrected = "Multiply the user-provided distance in miles by 1.609344 and display the result in kilometers."
	authority := directCodingRequirementGenerationAuthorityForRequest(t, request)
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
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				var input assemblyline.ApplicationRequirementCandidateResultRelationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				switch input.Candidate {
				case vague:
					candidate = assemblyline.ApplicationRequirementMissingResultRelation
				case corrected:
					candidate = assemblyline.ApplicationRequirementExplicitResultRelation
				default:
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected result candidate %q", input.Candidate)
				}
			case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
				var input assemblyline.ApplicationRequirementCandidateResultRelationGroundingInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if !reflect.DeepEqual(input.Context, authority.Authority.Context) {
					return assemblyline.PortableResult{}, fmt.Errorf("grounding lost application context authority")
				}
				candidate = assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
				var input assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.ImmutableRequest != request || input.CurrentCandidate != vague ||
					input.Defect != assemblyline.ApplicationRequirementMissingResultRelation ||
					!reflect.DeepEqual(input.Context, authority.Authority.Context) ||
					input.Grounding.Relation != assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed {
					return assemblyline.PortableResult{}, fmt.Errorf("correction received unbound authority: %+v", input)
				}
				for _, forbidden := range []string{
					"accepted_requirements", "excluded_candidates", "generation_authority",
				} {
					if strings.Contains(string(job.Payload), forbidden) {
						return assemblyline.PortableResult{}, fmt.Errorf(
							"correction leaked workflow authority %q", forbidden,
						)
					}
				}
				candidate = corrected
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", vague, authority,
		[]assemblyline.ApplicationIntentCandidateRequirement{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Retain || got.Candidate != corrected ||
		got.ResultRelation.Relation != assemblyline.ApplicationRequirementExplicitResultRelation ||
		got.ResultRelation.CandidateSHA256 != assemblyline.ExactObjectiveContextSHA(corrected) {
		t.Fatalf("resolution=%+v", got)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding,
		assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
	}
}

func TestDirectCodingRequirementFailsWhenResultRelationRemainsMissing(t *testing.T) {
	t.Parallel()
	const request = "Build a label formatter that converts user-provided text to Unicode lowercase."
	const vague = "Display the correctly formatted label."
	const stillVague = "Show an accurate transformed label."
	authority := directCodingRequirementGenerationAuthorityForRequest(t, request)
	correctionCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateKind:
				candidate = assemblyline.ApplicationRequirementCandidateTaskLocal
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				candidate = assemblyline.ApplicationRequirementMissingResultRelation
			case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
				candidate = assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
				correctionCalls++
				candidate = stillVague
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	_, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", vague, authority,
		[]assemblyline.ApplicationIntentCandidateRequirement{}, nil,
	)
	if err == nil || correctionCalls != 1 ||
		!strings.Contains(err.Error(), "remained underdetermined after its one correction") {
		t.Fatalf("corrections=%d error=%v", correctionCalls, err)
	}
}

func TestDirectCodingRequirementFailsLoudlyWhenRequestDoesNotGroundMissingResultRelation(
	t *testing.T,
) {
	t.Parallel()
	const request = "Build a browser preference assistant that displays a useful recommendation."
	const vague = "Accept preferences and display the best recommendation."
	authority := directCodingRequirementGenerationAuthorityForRequest(t, request)
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
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				candidate = assemblyline.ApplicationRequirementMissingResultRelation
			case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
				candidate = assemblyline.ApplicationRequirementNoExactlyOneDeterminingRelationEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
				return assemblyline.PortableResult{}, fmt.Errorf("correction opened without grounding authority")
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	_, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", vague, authority,
		[]assemblyline.ApplicationIntentCandidateRequirement{}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "refusing to invent result semantics") {
		t.Fatalf("negative grounding error=%v", err)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
	}
}

func TestDirectCodingRequirementCorrectionRoutesCodeOnlyWorkflowDuplicatesToZeroDelta(t *testing.T) {
	t.Parallel()
	const request = "Build a browser record normalizer that trims one submitted label and displays the normalized label."
	const vague = "Display the correct normalized label."
	const duplicate = "Trim surrounding whitespace from the submitted label and display it."
	tests := []struct {
		name     string
		accepted []string
		excluded []string
		set      string
	}{
		{name: "accepted", accepted: []string{duplicate}, excluded: []string{}, set: assemblyline.ApplicationRequirementZeroDeltaAcceptedSet},
		{name: "excluded", accepted: []string{}, excluded: []string{duplicate}, set: assemblyline.ApplicationRequirementZeroDeltaExcludedSet},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			authority := directCodingRequirementGenerationAuthorityWithCollections(
				t, request, test.accepted, test.excluded,
			)
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					candidate := ""
					switch job.Kind {
					case assemblyline.WorkApplicationRequirementCandidateKind:
						candidate = assemblyline.ApplicationRequirementCandidateTaskLocal
					case assemblyline.WorkApplicationRequirementCandidateCardinality:
						candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
					case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
						candidate = assemblyline.ApplicationRequirementDistinctRuntimeOutcomes
					case assemblyline.WorkApplicationRequirementCandidateResultRelation:
						candidate = assemblyline.ApplicationRequirementMissingResultRelation
					case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
						candidate = assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed
					case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
						if strings.Contains(string(job.Payload), duplicate) {
							return assemblyline.PortableResult{}, fmt.Errorf(
								"correction envelope leaked code-only workflow collection",
							)
						}
						candidate = duplicate
					default:
						return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			resolved, err := resolveDirectCodingApplicationRequirementCandidate(
				runtime, "intent-model", vague, authority,
				directCodingAcceptedRequirementAuthorities(t, test.accepted), nil,
			)
			if err != nil || resolved.Retain || resolved.ZeroDelta == nil ||
				resolved.ZeroDelta.RetainedSet != test.set {
				t.Fatalf("code-only %s duplicate resolution=%+v error=%v", test.name, resolved, err)
			}
		})
	}
}

func directCodingRequirementGenerationAuthorityForRequest(
	t testing.TB,
	request string,
) assemblyline.ApplicationRequirementCandidateInput {
	t.Helper()
	return directCodingRequirementGenerationAuthorityWithCollections(
		t, request, []string{}, []string{},
	)
}

func directCodingRequirementGenerationAuthorityWithCollections(
	t testing.TB,
	request string,
	accepted []string,
	excluded []string,
) assemblyline.ApplicationRequirementCandidateInput {
	t.Helper()
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.ApplicationRequirementCoverageInput{
		UserRequest: request, Context: applicationContext,
		AcceptedRequirements: append([]string{}, accepted...),
		ExcludedCandidates:   append([]string{}, excluded...),
		ZeroDeltas:           []assemblyline.ApplicationRequirementCandidateZeroDelta{},
	}
	coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
		input, assemblyline.ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirementCandidateInput{
		Authority: input, Coverage: coverage,
	}
}
