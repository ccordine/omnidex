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
			case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
				var input assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if !reflect.DeepEqual(input.GenerationAuthority, authority) ||
					input.CandidateAuthority.Candidate != vague ||
					input.ResultRelation.Relation != assemblyline.ApplicationRequirementMissingResultRelation {
					return assemblyline.PortableResult{}, fmt.Errorf("correction received unbound authority: %+v", input)
				}
				candidate = corrected
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", vague, authority, nil,
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
	const request = "Build a capacity display that computes remaining capacity as the limit minus the used amount."
	const vague = "Display the correct remaining capacity."
	const stillVague = "Show an accurate available amount."
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
		runtime, "intent-model", vague, authority, nil,
	)
	if err == nil || correctionCalls != 1 ||
		!strings.Contains(err.Error(), "remained underdetermined after its one correction") {
		t.Fatalf("corrections=%d error=%v", correctionCalls, err)
	}
}

func directCodingRequirementGenerationAuthorityForRequest(
	t testing.TB,
	request string,
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
		AcceptedRequirements: []string{}, ExcludedCandidates: []string{},
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
