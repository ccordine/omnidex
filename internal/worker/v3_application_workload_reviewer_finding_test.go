package worker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationJobSpecificationRepairActionsExactFindingAcrossUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name               string
		product            string
		requirement        string
		siblingRequirement string
		initialObjective   string
		behavior           string
		criterion          string
		finding            string
		findingEvidence    string
		invalidEvidence    string
		repairedObjective  string
	}{
		{
			name: "inventory filtering", product: "warehouse operations portal",
			requirement:        "Users can filter visible inventory by status.",
			siblingRequirement: "Users can export a visible inventory report.",
			initialObjective:   "Implement status filtering and report export.",
			behavior:           "Users select a status and see the visible inventory respond.",
			criterion:          "Selecting a status excludes inventory with other statuses.",
			finding: "The objective describes the whole product instead of the focused " +
				"status-filtering outcome.",
			findingEvidence:   "report export",
			invalidEvidence:   "bulk shipment scheduling",
			repairedObjective: "Implement status filtering for the visible inventory.",
		},
		{
			name: "appointment reminders", product: "appointment coordination portal",
			requirement:        "Users can dismiss a visible appointment reminder.",
			siblingRequirement: "Users can reschedule an appointment.",
			initialObjective:   "Implement reminder dismissal and appointment rescheduling.",
			behavior:           "Users dismiss one visible reminder and it is no longer shown.",
			criterion:          "Dismissing one reminder removes that reminder from view.",
			finding: "The objective describes the whole product instead of the focused " +
				"reminder-dismissal outcome.",
			findingEvidence:   "appointment rescheduling",
			invalidEvidence:   "calendar color customization",
			repairedObjective: "Implement dismissal of a visible appointment reminder.",
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			requirement := assemblyline.Requirement{
				ID: "requirement_001", SourceQuote: fixture.requirement,
			}
			sibling := assemblyline.Requirement{
				ID: "requirement_002", SourceQuote: fixture.siblingRequirement,
			}
			authority := assemblyline.ApplicationJobSpecificationInput{
				Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: fixture.product,
				AcceptedRequirements: []assemblyline.Requirement{requirement, sibling},
				FocusedRequirement:   requirement,
			}
			var kinds []assemblyline.WorkKind
			reviews := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				Execute: func(
					job assemblyline.PortableJob,
					_ string,
				) (assemblyline.PortableResult, error) {
					prompt, schema, err := assemblyline.RenderPortableJob(job)
					if err != nil {
						return assemblyline.PortableResult{}, err
					}
					kinds = append(kinds, job.Kind)
					switch job.Kind {
					case assemblyline.WorkApplicationJobSpecification:
						return workloadPortableCandidate(job, fmt.Sprintf(
							`{"objective":%q,"required_behaviors":[%q],"acceptance_criteria":[%q]}`,
							fixture.initialObjective, fixture.behavior, fixture.criterion,
						)), nil
					case assemblyline.WorkApplicationJobSpecificationReview:
						reviews++
						if reviews == 1 {
							return workloadPortableCandidate(job, fmt.Sprintf(
								`{"decision":"repair","field":"objective","finding":%q,"finding_evidence":%q}`,
								fixture.finding, fixture.invalidEvidence,
							)), nil
						}
						if reviews == 2 {
							for _, exact := range []string{
								`"prior_validation_failure"`, fixture.invalidEvidence,
								fixture.initialObjective,
							} {
								if !strings.Contains(prompt, exact) {
									return assemblyline.PortableResult{}, fmt.Errorf(
										"review retry omitted %q", exact,
									)
								}
							}
							return workloadPortableCandidate(job, fmt.Sprintf(
								`{"decision":"repair","field":"objective","finding":%q,"finding_evidence":%q}`,
								fixture.finding, fixture.findingEvidence,
							)), nil
						}
						if !strings.Contains(prompt, fixture.repairedObjective) {
							return assemblyline.PortableResult{}, fmt.Errorf(
								"review did not receive repaired objective",
							)
						}
						return workloadPortableCandidate(job, `{"decision":"accept"}`), nil
					case assemblyline.WorkApplicationJobSpecificationRepair:
						for _, exact := range []string{
							fixture.finding, fixture.findingEvidence, fixture.initialObjective,
							`"review_finding"`, `"finding_evidence"`, `"current_derived_value"`,
						} {
							if !strings.Contains(prompt, exact) {
								return assemblyline.PortableResult{}, fmt.Errorf(
									"repair prompt omitted %q", exact,
								)
							}
						}
						for _, unrelated := range []string{
							fixture.product,
							fixture.siblingRequirement,
							fixture.behavior,
							fixture.criterion,
						} {
							if strings.Contains(prompt, unrelated) {
								return assemblyline.PortableResult{}, fmt.Errorf(
									"repair prompt leaked retained unrelated field %q", unrelated,
								)
							}
						}
						properties := schema["properties"].(map[string]any)
						if len(properties) != 1 || properties["objective"] == nil {
							return assemblyline.PortableResult{}, fmt.Errorf(
								"repair schema is not objective-only: %v", schema,
							)
						}
						return workloadPortableCandidate(job, fmt.Sprintf(
							`{"objective":%q}`, fixture.repairedObjective,
						)), nil
					default:
						return assemblyline.PortableResult{}, fmt.Errorf(
							"unexpected work kind %s", job.Kind,
						)
					}
				},
			}

			got, err := resolveDirectCodingApplicationJobSpecification(
				runtime, "planner", "reviewer", "fixture", authority,
			)
			if err != nil {
				t.Fatal(err)
			}
			wantKinds := []assemblyline.WorkKind{
				assemblyline.WorkApplicationJobSpecification,
				assemblyline.WorkApplicationJobSpecificationReview,
				assemblyline.WorkApplicationJobSpecificationReview,
				assemblyline.WorkApplicationJobSpecificationRepair,
				assemblyline.WorkApplicationJobSpecificationReview,
			}
			if !reflect.DeepEqual(kinds, wantKinds) || got.Objective != fixture.repairedObjective {
				t.Fatalf("kinds=%v objective=%q", kinds, got.Objective)
			}
		})
	}
}

func TestApplicationJobSpecificationReviewStopsRepeatedUngroundedEvidenceWithoutRepair(t *testing.T) {
	t.Parallel()
	requirement := assemblyline.Requirement{
		ID: "requirement_001", SourceQuote: "Users can archive a completed invoice.",
	}
	authority := assemblyline.ApplicationJobSpecificationInput{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "billing portal",
		AcceptedRequirements: []assemblyline.Requirement{requirement},
		FocusedRequirement:   requirement,
	}
	var kinds []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			kinds = append(kinds, job.Kind)
			switch job.Kind {
			case assemblyline.WorkApplicationJobSpecification:
				return workloadPortableCandidate(job, `{"objective":"Implement invoice archival.","required_behaviors":["Users archive one completed invoice."],"acceptance_criteria":["The archived invoice is no longer active."]}`), nil
			case assemblyline.WorkApplicationJobSpecificationReview:
				return workloadPortableCandidate(job, `{"decision":"repair","field":"objective","finding":"The objective includes unrelated payment collection.","finding_evidence":"payment collection"}`), nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf(
					"ungrounded finding dispatched forbidden work %s", job.Kind,
				)
			}
		},
	}

	_, err := resolveDirectCodingApplicationJobSpecification(
		runtime, "planner", "reviewer", "fixture", authority,
	)
	if err == nil || !strings.Contains(err.Error(), "repeated ungrounded review evidence") {
		t.Fatalf("repeated reviewer evidence error=%v", err)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkApplicationJobSpecification,
		assemblyline.WorkApplicationJobSpecificationReview,
		assemblyline.WorkApplicationJobSpecificationReview,
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("calls=%v want re-review without repair %v", kinds, want)
	}
}
