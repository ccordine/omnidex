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
			findingEvidence:   "Implement status filtering and report export.",
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
			findingEvidence:   "Implement reminder dismissal and appointment rescheduling.",
			invalidEvidence:   "reminder dismissal\nappointment rescheduling",
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
					model string,
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
							encodedInvalidEvidence, encodeErr := json.Marshal(fixture.invalidEvidence)
							if encodeErr != nil {
								return assemblyline.PortableResult{}, encodeErr
							}
							for _, exact := range []string{
								`"prior_validation_failure"`, string(encodedInvalidEvidence),
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
						if model != "reviewer" {
							return assemblyline.PortableResult{}, fmt.Errorf("repair model=%q", model)
						}
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

func TestApplicationJobSpecificationRepairNoOpGetsOneBoundedCorrectionAcrossUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name                  string
		product               string
		requirement           string
		siblingRequirement    string
		targetField           string
		initialCandidate      string
		finding               string
		findingEvidence       string
		noOpCandidate         string
		correctionCandidate   string
		repairedMarker        string
		unrelatedRetainedText string
	}{
		{
			name:               "inventory archival objective",
			product:            "inventory operations portal",
			requirement:        "Users can archive one completed inventory count.",
			siblingRequirement: "Users can export an inventory report.",
			targetField:        "objective",
			initialCandidate: `{"objective":"Implement completed-count archival and report export.",` +
				`"required_behaviors":["Users archive one completed inventory count."],` +
				`"acceptance_criteria":["The archived count is no longer active."]}`,
			finding:               "The objective includes unrelated report export work.",
			findingEvidence:       "Implement completed-count archival and report export.",
			noOpCandidate:         `{"objective":"Implement completed-count archival and report export."}`,
			correctionCandidate:   `{"objective":"Implement archival of one completed inventory count."}`,
			repairedMarker:        "Implement archival of one completed inventory count.",
			unrelatedRetainedText: "The archived count is no longer active.",
		},
		{
			name:               "appointment reminder behavior",
			product:            "appointment coordination portal",
			requirement:        "Users can dismiss one visible appointment reminder.",
			siblingRequirement: "Users can reschedule an appointment.",
			targetField:        "required_behaviors",
			initialCandidate: `{"objective":"Implement dismissal of one visible appointment reminder.",` +
				`"required_behaviors":["Users dismiss one visible reminder.","Users reschedule an appointment."],` +
				`"acceptance_criteria":["The dismissed reminder is no longer visible."]}`,
			finding:               "The required behaviors include unrelated appointment rescheduling.",
			findingEvidence:       "Users reschedule an appointment.",
			noOpCandidate:         `{"required_behaviors":["Users dismiss one visible reminder.","Users reschedule an appointment."]}`,
			correctionCandidate:   `{"required_behaviors":["Users dismiss one visible reminder."]}`,
			repairedMarker:        "Users dismiss one visible reminder.",
			unrelatedRetainedText: "The dismissed reminder is no longer visible.",
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
					model string,
				) (assemblyline.PortableResult, error) {
					prompt, schema, err := assemblyline.RenderPortableJob(job)
					if err != nil {
						return assemblyline.PortableResult{}, err
					}
					kinds = append(kinds, job.Kind)
					switch job.Kind {
					case assemblyline.WorkApplicationJobSpecification:
						return workloadPortableCandidate(job, fixture.initialCandidate), nil
					case assemblyline.WorkApplicationJobSpecificationReview:
						reviews++
						if reviews == 1 {
							return workloadPortableCandidate(job, fmt.Sprintf(
								`{"decision":"repair","field":%q,"finding":%q,"finding_evidence":%q}`,
								fixture.targetField, fixture.finding, fixture.findingEvidence,
							)), nil
						}
						if !strings.Contains(prompt, fixture.repairedMarker) {
							return assemblyline.PortableResult{}, fmt.Errorf(
								"review did not receive corrected repair state",
							)
						}
						return workloadPortableCandidate(job, `{"decision":"accept"}`), nil
					case assemblyline.WorkApplicationJobSpecificationRepair:
						if model != "reviewer" {
							return assemblyline.PortableResult{}, fmt.Errorf("repair model=%q", model)
						}
						return workloadPortableCandidate(job, fixture.noOpCandidate), nil
					case assemblyline.WorkResponseCorrection:
						if model != "reviewer" {
							return assemblyline.PortableResult{}, fmt.Errorf("correction model=%q", model)
						}
						for _, exact := range []string{
							fixture.requirement,
							fixture.finding,
							fixture.findingEvidence,
							"application job specification repair is a no-op",
						} {
							if !strings.Contains(prompt, exact) {
								return assemblyline.PortableResult{}, fmt.Errorf(
									"repair correction prompt omitted %q", exact,
								)
							}
						}
						for _, unrelated := range []string{
							fixture.product,
							fixture.siblingRequirement,
							fixture.unrelatedRetainedText,
						} {
							if strings.Contains(prompt, unrelated) {
								return assemblyline.PortableResult{}, fmt.Errorf(
									"repair correction leaked unrelated authority %q", unrelated,
								)
							}
						}
						properties, _ := schema["properties"].(map[string]any)
						if len(properties) != 1 || properties[fixture.targetField] == nil {
							return assemblyline.PortableResult{}, fmt.Errorf(
								"repair correction schema=%v", schema,
							)
						}
						return workloadPortableCandidate(job, fixture.correctionCandidate), nil
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
				assemblyline.WorkApplicationJobSpecificationRepair,
				assemblyline.WorkResponseCorrection,
				assemblyline.WorkApplicationJobSpecificationReview,
			}
			if !reflect.DeepEqual(kinds, wantKinds) {
				t.Fatalf("calls=%v want %v", kinds, wantKinds)
			}
			if fixture.targetField == "objective" && got.Objective != fixture.repairedMarker {
				t.Fatalf("objective=%q", got.Objective)
			}
			if fixture.targetField == "required_behaviors" &&
				(len(got.RequiredBehaviors) != 1 || got.RequiredBehaviors[0] != fixture.repairedMarker) {
				t.Fatalf("required_behaviors=%v", got.RequiredBehaviors)
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
	if err == nil || !strings.Contains(err.Error(), "repeated invalid review evidence") {
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
