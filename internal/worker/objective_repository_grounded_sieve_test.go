package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestRepositoryGroundedParagraphQueueRestoresEveryExactLeafWithoutProviderCalls(t *testing.T) {
	t.Parallel()
	const (
		accepted    = "The registry owns the active lease duration."
		unsupported = "A wall clock owns the active lease duration."
	)
	input := assemblyline.GroundedAnswerInput{
		RequirementID:    "lease-owner",
		ExactRequirement: "Which component owns the active lease duration?",
		Evidence: []assemblyline.GroundedEvidenceCapsule{{
			ID: "registry", Text: "LeaseRegistry stores the active lease duration.",
		}},
	}
	var kinds []assemblyline.WorkKind
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		kinds = append(kinds, request.Job.Kind)
		var raw string
		switch request.Job.Kind {
		case assemblyline.WorkGroundedAnswerParagraphInventory:
			raw = accepted + "\n" + unsupported + "\n" + accepted
		case assemblyline.WorkGroundedAnswerParagraphEvidenceRelation:
			var authority assemblyline.GroundedAnswerParagraphEvidenceRelationInput
			if err := json.Unmarshal(request.Job.Payload, &authority); err != nil {
				return queue.ObjectivePortableResultReuse{}, false, err
			}
			raw = string(assemblyline.GroundedEvidenceSupportsParagraph)
			if authority.ParagraphText == unsupported {
				raw = string(assemblyline.GroundedEvidenceDoesNotSupport)
			}
		case assemblyline.WorkGroundedAnswerParagraphAuthorization:
			var authority assemblyline.GroundedAnswerParagraphAuthorizationInput
			if err := json.Unmarshal(request.Job.Payload, &authority); err != nil {
				return queue.ObjectivePortableResultReuse{}, false, err
			}
			raw = string(assemblyline.GroundedParagraphResponsiveAndFullySupported)
			if authority.ParagraphText == unsupported {
				raw = string(assemblyline.GroundedParagraphNotResponsiveOrUnsupported)
			}
		default:
			t.Fatalf("unexpected restored grounded-answer work kind %q", request.Job.Kind)
		}
		projection, err := assemblyline.NewExactPortableResultProjection(raw)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: raw, Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	decision, receipt, err := (&portableObjectiveRepositoryGroundingStation{runtime: runtime}).Answer(
		t.Context(), input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Text != accepted || !reflect.DeepEqual(decision.EvidenceIDs, []string{"registry"}) {
		t.Fatalf("decision=%#v", decision)
	}
	if receipt.Calls != 0 || !receipt.Reused {
		t.Fatalf("receipt=%+v", receipt)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkGroundedAnswerParagraphInventory,
		assemblyline.WorkGroundedAnswerParagraphAuthorization,
		assemblyline.WorkGroundedAnswerParagraphEvidenceRelation,
		assemblyline.WorkGroundedAnswerParagraphAuthorization,
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("restored kinds=%v want=%v", kinds, wantKinds)
	}
}

func TestRepositoryGroundedParagraphQueueSievesUnrelatedFixturesIndependently(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         assemblyline.GroundedAnswerInput
		candidates    []string
		support       map[string]map[string]bool
		authorized    map[string]bool
		wantText      string
		wantEvidence  []string
		wantCalls     int
		wantSupport   int
		wantAuthorize []string
	}{
		{
			name: "repository dispatch ownership",
			input: assemblyline.GroundedAnswerInput{
				RequirementID:    "dispatch-owner",
				ExactRequirement: "Which component owns invitation timing?",
				Evidence: []assemblyline.GroundedEvidenceCapsule{
					{ID: "dispatch-config", Text: "DeliveryConfig declares the dispatch interval."},
					{ID: "scheduler", Text: "InvitationScheduler reads the configured interval."},
				},
			},
			candidates: []string{
				"DeliveryConfig owns the invitation timing setting.",
				"A lunar clock secretly controls invitations.",
				"DeliveryConfig owns the invitation timing setting.",
				"InvitationScheduler sends invitations immediately.",
			},
			support: map[string]map[string]bool{
				"DeliveryConfig owns the invitation timing setting.": {"dispatch-config": true},
				"InvitationScheduler sends invitations immediately.": {"scheduler": true},
			},
			authorized: map[string]bool{
				"DeliveryConfig owns the invitation timing setting.": true,
			},
			wantText:     "DeliveryConfig owns the invitation timing setting.",
			wantEvidence: []string{"dispatch-config"},
			wantCalls:    6,
			wantSupport:  2,
			wantAuthorize: []string{
				"DeliveryConfig owns the invitation timing setting.",
				"A lunar clock secretly controls invitations.",
				"InvitationScheduler sends invitations immediately.",
			},
		},
		{
			name: "greenhouse moisture guidance",
			input: assemblyline.GroundedAnswerInput{
				RequirementID:    "greenhouse-water",
				ExactRequirement: "Explain when the greenhouse beds need water.",
				Evidence: []assemblyline.GroundedEvidenceCapsule{
					{ID: "sensor", Text: "The moisture sensor reports 18 percent."},
					{ID: "threshold", Text: "Beds require watering below 22 percent moisture."},
					{ID: "forecast", Text: "The greenhouse roof prevents forecast rain from reaching the beds."},
				},
			},
			candidates: []string{
				"The beds need water because measured moisture is below the watering threshold.",
				"Forecast rain will not water the covered beds.",
				"Forecast rain will not water the covered beds.",
			},
			support: map[string]map[string]bool{
				"The beds need water because measured moisture is below the watering threshold.": {"sensor": true, "threshold": true},
				"Forecast rain will not water the covered beds.":                                 {"forecast": true},
			},
			authorized: map[string]bool{
				"The beds need water because measured moisture is below the watering threshold.": true,
				"Forecast rain will not water the covered beds.":                                 true,
			},
			wantText: "The beds need water because measured moisture is below the watering threshold.\n\n" +
				"Forecast rain will not water the covered beds.",
			wantEvidence:  []string{"sensor", "threshold", "forecast"},
			wantCalls:     9,
			wantSupport:   6,
			wantAuthorize: []string{"The beds need water because measured moisture is below the watering threshold.", "Forecast rain will not water the covered beds."},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var supportInputs []assemblyline.GroundedAnswerParagraphEvidenceRelationInput
			var authorizationInputs []assemblyline.GroundedAnswerParagraphAuthorizationInput
			decision, receipt, err := resolveRepositoryGroundedParagraphQueue(
				t.Context(),
				test.input,
				func(
					_ context.Context,
					input assemblyline.GroundedAnswerParagraphInventoryInput,
				) (assemblyline.GroundedAnswerParagraphInventory, objectiveStationReceipt, error) {
					inventory, err := assemblyline.DecodeGroundedAnswerParagraphInventory(
						input, strings.Join(test.candidates, "\n"),
					)
					return inventory, objectiveStationReceipt{Calls: 1}, err
				},
				func(
					_ context.Context,
					input assemblyline.GroundedAnswerParagraphEvidenceRelationInput,
				) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, objectiveStationReceipt, error) {
					supportInputs = append(supportInputs, input)
					relation := assemblyline.GroundedEvidenceDoesNotSupport
					if test.support[input.ParagraphText][input.Evidence.ID] {
						relation = assemblyline.GroundedEvidenceSupportsParagraph
					}
					return assemblyline.GroundedAnswerParagraphEvidenceRelationDecision{
						Relation: relation,
					}, objectiveStationReceipt{Calls: 1}, nil
				},
				func(
					_ context.Context,
					input assemblyline.GroundedAnswerParagraphAuthorizationInput,
				) (assemblyline.GroundedAnswerParagraphAuthorizationDecision, objectiveStationReceipt, error) {
					authorizationInputs = append(authorizationInputs, input)
					relation := assemblyline.GroundedParagraphNotResponsiveOrUnsupported
					if test.authorized[input.ParagraphText] {
						relation = assemblyline.GroundedParagraphResponsiveAndFullySupported
					}
					return assemblyline.GroundedAnswerParagraphAuthorizationDecision{
						Relation: relation,
					}, objectiveStationReceipt{Calls: 1}, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Text != test.wantText || !reflect.DeepEqual(decision.EvidenceIDs, test.wantEvidence) {
				t.Fatalf("decision=%#v", decision)
			}
			if receipt.Calls != test.wantCalls || len(supportInputs) != test.wantSupport {
				t.Fatalf("receipt=%+v support calls=%d", receipt, len(supportInputs))
			}
			gotAuthorized := make([]string, len(authorizationInputs))
			for index, input := range authorizationInputs {
				gotAuthorized[index] = input.ParagraphText
				if !reflect.DeepEqual(input.Evidence, test.input.Evidence) {
					t.Fatalf("authorization evidence=%#v want=%#v", input.Evidence, test.input.Evidence)
				}
			}
			if !reflect.DeepEqual(gotAuthorized, test.wantAuthorize) {
				t.Fatalf("authorized candidates=%v want=%v", gotAuthorized, test.wantAuthorize)
			}
		})
	}
}

func TestRepositoryGroundedParagraphQueueFailsExplicitlyAfterEmptyInventoryExhaustion(t *testing.T) {
	t.Parallel()
	input := assemblyline.GroundedAnswerInput{
		RequirementID: "empty", ExactRequirement: "State the supported fact.",
		Evidence: []assemblyline.GroundedEvidenceCapsule{{ID: "one", Text: "Unrelated evidence."}},
	}
	decision, receipt, err := resolveRepositoryGroundedParagraphQueue(
		t.Context(),
		input,
		func(
			_ context.Context,
			leafInput assemblyline.GroundedAnswerParagraphInventoryInput,
		) (assemblyline.GroundedAnswerParagraphInventory, objectiveStationReceipt, error) {
			inventory, err := assemblyline.DecodeGroundedAnswerParagraphInventory(
				leafInput, assemblyline.GroundedAnswerNoParagraphCandidates,
			)
			return inventory, objectiveStationReceipt{Calls: 1}, err
		},
		func(context.Context, assemblyline.GroundedAnswerParagraphEvidenceRelationInput) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, objectiveStationReceipt, error) {
			t.Fatal("empty inventory invoked support relation")
			return assemblyline.GroundedAnswerParagraphEvidenceRelationDecision{}, objectiveStationReceipt{}, nil
		},
		func(context.Context, assemblyline.GroundedAnswerParagraphAuthorizationInput) (assemblyline.GroundedAnswerParagraphAuthorizationDecision, objectiveStationReceipt, error) {
			t.Fatal("empty inventory invoked authorization")
			return assemblyline.GroundedAnswerParagraphAuthorizationDecision{}, objectiveStationReceipt{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "produced no responsive fully supported paragraphs") {
		t.Fatalf("decision=%#v receipt=%+v error=%v", decision, receipt, err)
	}
	if receipt.Calls != 1 {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestRepositoryGroundedParagraphQueueMarksMixedRestoreAsFreshAggregate(t *testing.T) {
	t.Parallel()
	input := assemblyline.GroundedAnswerInput{
		RequirementID: "mixed", ExactRequirement: "Which store owns the lease?",
		Evidence: []assemblyline.GroundedEvidenceCapsule{{
			ID: "lease-store", Text: "LeaseStore owns the lease.",
		}},
	}
	decision, receipt, err := resolveRepositoryGroundedParagraphQueue(
		t.Context(),
		input,
		func(
			_ context.Context,
			leafInput assemblyline.GroundedAnswerParagraphInventoryInput,
		) (assemblyline.GroundedAnswerParagraphInventory, objectiveStationReceipt, error) {
			inventory, err := assemblyline.DecodeGroundedAnswerParagraphInventory(
				leafInput, "LeaseStore owns the lease.",
			)
			return inventory, objectiveStationReceipt{Reused: true}, err
		},
		func(
			context.Context,
			assemblyline.GroundedAnswerParagraphEvidenceRelationInput,
		) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, objectiveStationReceipt, error) {
			return assemblyline.GroundedAnswerParagraphEvidenceRelationDecision{
				Relation: assemblyline.GroundedEvidenceSupportsParagraph,
			}, objectiveStationReceipt{Calls: 1}, nil
		},
		func(
			context.Context,
			assemblyline.GroundedAnswerParagraphAuthorizationInput,
		) (assemblyline.GroundedAnswerParagraphAuthorizationDecision, objectiveStationReceipt, error) {
			return assemblyline.GroundedAnswerParagraphAuthorizationDecision{
				Relation: assemblyline.GroundedParagraphResponsiveAndFullySupported,
			}, objectiveStationReceipt{Reused: true}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Text != "LeaseStore owns the lease." || receipt.Calls != 1 || receipt.Reused {
		t.Fatalf("decision=%+v receipt=%+v", decision, receipt)
	}
}
