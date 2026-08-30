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

func TestDirectCodingRequirementCorrectsOneMissingResultRelation(t *testing.T) {
	t.Parallel()
	const request = "Build a browser distance converter that converts miles to kilometers using 1 mile = 1.609344 kilometers."
	const vague = "Accept a distance and display an accurate converted result."
	const corrected = "Multiply the submitted distance in miles by 1.609344 and display the result in kilometers."
	authority, entry := directCodingRequirementQueueEntry(t, request, vague, nil)
	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var err error
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
					job,
					assemblyline.ApplicationRequirementCandidateTaskLocal,
				)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				var input assemblyline.ApplicationRequirementCandidateResultRelationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == vague {
					candidate = assemblyline.ApplicationRequirementMissingResultRelation
				} else if input.Candidate == corrected {
					candidate = assemblyline.ApplicationRequirementExplicitResultRelation
				} else {
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected candidate %q", input.Candidate)
				}
			case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
				candidate = assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
				candidate = corrected
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	got, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", authority, entry, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Disposition != directCodingApplicationRequirementRetained ||
		got.Candidate != corrected ||
		got.ResultRelation.Relation != assemblyline.ApplicationRequirementExplicitResultRelation {
		t.Fatalf("resolution=%+v", got)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateAuthorization,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding,
		assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection,
		assemblyline.WorkApplicationRequirementCandidateAuthorization,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
	}
}

func TestDirectCodingRequirementMissingResultRelationDiscardsOnlyThatCandidate(
	t *testing.T,
) {
	t.Parallel()
	tests := []struct {
		name            string
		grounding       string
		correction      string
		wantCorrections int
	}{
		{
			name:            "request does not determine result",
			grounding:       assemblyline.ApplicationRequirementNoExactlyOneDeterminingRelationEntailed,
			wantCorrections: 0,
		},
		{
			name:            "one correction remains vague",
			grounding:       assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed,
			correction:      "Show an accurate transformed label.",
			wantCorrections: 1,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			const request = "Build a label formatter that converts user-provided text to Unicode lowercase."
			const vague = "Display the correctly formatted label."
			authority, entry := directCodingRequirementQueueEntry(t, request, vague, nil)
			corrections := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					candidate := ""
					switch job.Kind {
					case assemblyline.WorkApplicationRequirementCandidateKind:
						var err error
						candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
							job,
							assemblyline.ApplicationRequirementCandidateTaskLocal,
						)
						if err != nil {
							return assemblyline.PortableResult{}, err
						}
					case assemblyline.WorkApplicationRequirementCandidateCardinality:
						candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
					case assemblyline.WorkApplicationRequirementCandidateAuthorization:
						candidate = assemblyline.ApplicationRequirementCandidateEntailed
					case assemblyline.WorkApplicationRequirementCandidateResultRelation:
						candidate = assemblyline.ApplicationRequirementMissingResultRelation
					case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
						candidate = test.grounding
					case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
						corrections++
						candidate = test.correction
					default:
						return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
					}
					return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
				},
			}
			resolved, err := resolveDirectCodingApplicationRequirementCandidate(
				runtime, "intent-model", authority, entry, nil, nil, nil,
			)
			if err != nil || resolved.Disposition != directCodingApplicationRequirementUnresolved {
				t.Fatalf("corrections=%d resolution=%+v error=%v", corrections, resolved, err)
			}
			if corrections != test.wantCorrections {
				t.Fatalf("corrections=%d want=%d", corrections, test.wantCorrections)
			}
		})
	}
}

func TestDirectCodingRequirementCorrectionPreservesVerifiedContext(t *testing.T) {
	t.Parallel()
	const request = "Build a browser measurement converter using the verified conversion policy."
	const fact = "The verified policy multiplies the submitted yard value by 3 and reports feet."
	const vague = "Accept a measurement and display the correct converted result."
	const corrected = "Multiply the submitted yard value by 3 and display the result in feet."
	authority, entry := directCodingRequirementQueueEntry(t, request, vague, func(context *assemblyline.ApplicationContext) {
		context.Facts = append(context.Facts, assemblyline.ApplicationContextFact{
			ID: "fact_002", Kind: assemblyline.ApplicationContextRepositoryFact,
			Authority: assemblyline.ApplicationContextEvidenceAuthority,
			NeedID:    "verified_context_need", Value: fact,
			SourceID: "verified_context_source", SourceSHA256: assemblyline.ExactObjectiveContextSHA(fact),
		})
	})
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var err error
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
					job,
					assemblyline.ApplicationRequirementCandidateTaskLocal,
				)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				var input assemblyline.ApplicationRequirementCandidateResultRelationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == vague {
					candidate = assemblyline.ApplicationRequirementMissingResultRelation
				} else {
					candidate = assemblyline.ApplicationRequirementExplicitResultRelation
				}
			case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
				prompt, err := assemblyline.RenderPortableJob(job)
				if err != nil || !strings.Contains(prompt, fact) || strings.Contains(prompt, "verified_context_source") {
					return assemblyline.PortableResult{}, fmt.Errorf("grounding lost minimal verified context: %v", err)
				}
				candidate = assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
				candidate = corrected
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	resolved, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", authority, entry, nil, nil, nil,
	)
	if err != nil || resolved.Candidate != corrected {
		t.Fatalf("resolution=%+v error=%v", resolved, err)
	}
}

func directCodingRequirementQueueEntry(
	t testing.TB,
	request string,
	candidate string,
	mutateContext func(*assemblyline.ApplicationContext),
) (assemblyline.ApplicationRequirementInventoryInput, directCodingApplicationRequirementCandidateQueueEntry) {
	t.Helper()
	workspaceState := assemblyline.ApplicationWorkspaceEmpty
	if mutateContext != nil {
		workspaceState = assemblyline.ApplicationWorkspaceExisting
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(request, workspaceState)
	if err != nil {
		t.Fatal(err)
	}
	if mutateContext != nil {
		mutateContext(&applicationContext)
	}
	input := assemblyline.ApplicationRequirementInventoryInput{
		UserRequest: request,
		Context:     applicationContext,
	}
	inventory, err := assemblyline.DecodeApplicationRequirementInventory(input, candidate)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := newDirectCodingApplicationRequirementCandidateQueue(input, inventory)
	if err != nil {
		t.Fatal(err)
	}
	return input, queue[0]
}
