package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingResultRelationUsesSecondQuestionOnlyForDerivedValues(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name, candidate, relation string
		wantCalls                 int
	}{
		{
			name:      "ordered records",
			candidate: "The finished software orders supplied records by ascending timestamp.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
			wantCalls: 2,
		},
		{
			name:      "content digest",
			candidate: "The finished software computes the SHA-256 digest of supplied content bytes.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
			wantCalls: 2,
		},
		{
			name:      "measurement operation family",
			candidate: "The finished software performs unit-conversion operations on supplied measurements.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
			wantCalls: 2,
		},
		{
			name:      "observation operation family",
			candidate: "The finished software performs statistical aggregation operations on supplied observations.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
			wantCalls: 2,
		},
		{
			name:      "spatial property measurement",
			candidate: "The finished software reports the dimensions of each transformed image.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
			wantCalls: 2,
		},
		{
			name:      "collection property measurement",
			candidate: "The finished software reports the item count of each supplied batch.",
			relation:  assemblyline.ApplicationRequirementExplicitResultRelation,
			wantCalls: 2,
		},
		{
			name:      "status control",
			candidate: "The finished software shows one status control.",
			relation:  assemblyline.ApplicationRequirementNoDerivedResult,
			wantCalls: 1,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			authority := directCodingResultRelationAuthorityFixture(t, fixture.candidate)
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(job assemblyline.PortableJob, requestedModel string) (assemblyline.PortableResult, error) {
					calls++
					if job.Kind != assemblyline.WorkApplicationRequirementCandidateResultRelation {
						return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
					}
					if requestedModel != "result-model" {
						return assemblyline.PortableResult{}, fmt.Errorf(
							"result relation model=%q",
							requestedModel,
						)
					}
					var input assemblyline.ApplicationRequirementCandidateResultPresenceInput
					if err := json.Unmarshal(job.Payload, &input); err != nil {
						return assemblyline.PortableResult{}, err
					}
					candidate := string(assemblyline.ApplicationRequirementCandidateResultAbsent)
					if fixture.relation == assemblyline.ApplicationRequirementExplicitResultRelation {
						candidate = string(assemblyline.ApplicationRequirementCandidateResultPresent)
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			result, err := classifyDirectCodingApplicationRequirementCandidateResultRelation(
				runtime, "result-model", fixture.candidate,
				authority.Kind, authority.Cardinality, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Relation != fixture.relation || calls != fixture.wantCalls {
				t.Fatalf("relation=%q calls=%d", result.Relation, calls)
			}
		})
	}
}

func TestDirectCodingUnderdeterminedResultIsGroundedThenDiscarded(t *testing.T) {
	t.Parallel()
	const request = "Build a material routing aid that selects the best destination for supplied material."
	const candidate = "The finished software selects the best destination for supplied material."
	applicationContext, err := assemblyline.BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	authority := assemblyline.ApplicationRequirementInventoryInput{
		UserRequest: request, Context: applicationContext,
	}
	entry := directCodingApplicationRequirementCandidateQueueEntry{Candidate: candidate}
	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, requestedModel string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			wantModel := "intent-model"
			if job.Kind == assemblyline.WorkApplicationRequirementCandidateResultRelation {
				wantModel = "result-model"
			}
			if requestedModel != wantModel {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"work kind %s used model %q, want %q",
					job.Kind,
					requestedModel,
					wantModel,
				)
			}
			response := ""
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				response = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var input assemblyline.ApplicationRequirementCandidateContentPresenceInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				response = string(assemblyline.ApplicationRequirementCandidateContentPresent)
				if input.Dimension == assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension {
					response = string(assemblyline.ApplicationRequirementCandidateContentAbsent)
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				response = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				var input assemblyline.ApplicationRequirementCandidateResultPresenceInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				response = string(assemblyline.ApplicationRequirementCandidateResultPresent)
				if input.Dimension == assemblyline.ApplicationRequirementDeterminingRelationDimension {
					response = string(assemblyline.ApplicationRequirementCandidateResultAbsent)
				}
			case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
				response = assemblyline.ApplicationRequirementNoExactlyOneDeterminingRelationEntailed
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: response}, nil
		},
	}
	resolved, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", "result-model", authority, entry, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Disposition != directCodingApplicationRequirementUnresolved ||
		resolved.ResultRelation != (assemblyline.ApplicationRequirementCandidateResultRelationResult{}) {
		t.Fatalf("resolution=%+v", resolved)
	}
	wantTail := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding,
	}
	if len(calls) < len(wantTail) || !reflect.DeepEqual(calls[len(calls)-len(wantTail):], wantTail) {
		t.Fatalf("calls=%v", calls)
	}
	for _, call := range calls {
		if call == assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection {
			t.Fatalf("negative grounding opened a correction: %v", calls)
		}
	}
}

func directCodingResultRelationAuthorityFixture(
	t testing.TB,
	candidate string,
) assemblyline.ApplicationRequirementCandidateResultRelationInput {
	t.Helper()
	runtimeInput := assemblyline.ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate,
		Dimension: assemblyline.ApplicationRequirementCandidateRuntimeContentDimension,
	}
	runtimeContent, err := assemblyline.DecodeApplicationRequirementCandidateContentPresenceResult(
		runtimeInput, string(assemblyline.ApplicationRequirementCandidateContentPresent),
	)
	if err != nil {
		t.Fatal(err)
	}
	nonRuntimeInput := assemblyline.ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate,
		Dimension: assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension,
	}
	nonRuntimeContent, err := assemblyline.DecodeApplicationRequirementCandidateContentPresenceResult(
		nonRuntimeInput, string(assemblyline.ApplicationRequirementCandidateContentAbsent),
	)
	if err != nil {
		t.Fatal(err)
	}
	kind, resolved, err := assemblyline.ResolveApplicationRequirementCandidateKind(
		candidate, runtimeContent, nonRuntimeContent,
	)
	if err != nil || !resolved {
		t.Fatalf("kind=%+v resolved=%t error=%v", kind, resolved, err)
	}
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		assemblyline.ApplicationRequirementCandidateCardinalityInput{Candidate: candidate},
		assemblyline.ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate, Kind: kind, Cardinality: cardinality,
	}
}
