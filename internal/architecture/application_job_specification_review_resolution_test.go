package architecture

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestSpecificationReviewRemovalIsCodeOwnedAcrossUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name        string
		product     string
		requirement string
		field       assemblyline.ApplicationJobSpecificationField
		evidenceID  string
		current     string
		retained    assemblyline.ApplicationJobSpecification
		want        assemblyline.ApplicationJobSpecification
	}{
		{
			name: "inventory behavior", product: "inventory console",
			requirement: "Users filter visible inventory by status.",
			field:       assemblyline.ApplicationJobSpecificationRequiredBehaviorsField,
			evidenceID:  "E2", current: "Users export a visible inventory report.",
			retained: assemblyline.ApplicationJobSpecification{
				Objective: "Implement status filtering.",
				RequiredBehaviors: []string{
					"Users select a status and see matching inventory.",
					"Users export a visible inventory report.",
				},
				AcceptanceCriteria: []string{"Selecting a status excludes other statuses."},
			},
			want: assemblyline.ApplicationJobSpecification{
				Objective:          "Implement status filtering.",
				RequiredBehaviors:  []string{"Users select a status and see matching inventory."},
				AcceptanceCriteria: []string{"Selecting a status excludes other statuses."},
			},
		},
		{
			name: "appointment criterion", product: "appointment portal",
			requirement: "Users dismiss one visible appointment reminder.",
			field:       assemblyline.ApplicationJobSpecificationAcceptanceCriteriaField,
			evidenceID:  "E2", current: "The appointment can be rescheduled.",
			retained: assemblyline.ApplicationJobSpecification{
				Objective:         "Implement reminder dismissal.",
				RequiredBehaviors: []string{"Users dismiss one visible reminder."},
				AcceptanceCriteria: []string{
					"The dismissed reminder is no longer visible.",
					"The appointment can be rescheduled.",
				},
			},
			want: assemblyline.ApplicationJobSpecification{
				Objective:         "Implement reminder dismissal.",
				RequiredBehaviors: []string{"Users dismiss one visible reminder."},
				AcceptanceCriteria: []string{
					"The dismissed reminder is no longer visible.",
				},
			},
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			requirement := assemblyline.Requirement{
				ID: "requirement_001", SourceQuote: fixture.requirement,
			}
			authority := assemblyline.ApplicationJobSpecificationInput{
				Surface:              assemblyline.ApplicationSurfaceBrowser,
				ProductQuote:         fixture.product,
				AcceptedRequirements: []assemblyline.Requirement{requirement},
				FocusedRequirement:   requirement,
			}
			reviewInput, err := assemblyline.NewApplicationJobSpecificationReviewInput(
				authority, fixture.retained, fixture.field, fixture.evidenceID, 1,
			)
			if err != nil {
				t.Fatal(err)
			}
			assertReviewSchemaAllowsRemoval(t, reviewInput)
			assertRepairContractCannotRemove(
				t, authority, fixture.retained, reviewInput, fixture.evidenceID,
			)
			review, err := assemblyline.DecodeApplicationJobSpecificationReview(
				reviewInput,
				fmt.Sprintf(
					`{"decision":"repair","resolution":"remove","evidence_id":%q,"finding":"The whole current value is outside the focused requirement."}`,
					fixture.evidenceID,
				),
			)
			if err != nil {
				t.Fatal(err)
			}
			updated, err := assemblyline.ApplyApplicationJobSpecificationReviewRemoval(
				authority, fixture.retained, review,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(updated, fixture.want) {
				t.Fatalf("updated=%#v want %#v", updated, fixture.want)
			}

			remaining := currentSpecificationFieldValue(t, updated, fixture.field)
			finalInput, err := assemblyline.NewApplicationJobSpecificationReviewInput(
				authority, updated, fixture.field, "E1", 2,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = assemblyline.DecodeApplicationJobSpecificationReview(
				finalInput,
				fmt.Sprintf(
					`{"decision":"repair","resolution":"remove","evidence_id":"E1","finding":"Remove %s."}`,
					remaining,
				),
			)
			if err == nil {
				t.Fatal("removing the final required list value was accepted")
			}
		})
	}
}

func assertRepairContractCannotRemove(
	t *testing.T,
	authority assemblyline.ApplicationJobSpecificationInput,
	retained assemblyline.ApplicationJobSpecification,
	reviewInput assemblyline.ApplicationJobSpecificationReviewInput,
	evidenceID string,
) {
	t.Helper()
	review, err := assemblyline.DecodeApplicationJobSpecificationReview(
		reviewInput,
		fmt.Sprintf(
			`{"decision":"repair","resolution":"replace","evidence_id":%q,"finding":"The current value belongs but must be corrected."}`,
			evidenceID,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	repairInput, err := assemblyline.NewApplicationJobSpecificationRepairInput(
		authority, retained, review, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := assemblyline.ApplicationJobSpecificationRepairResponseSchema(repairInput)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	field := properties[string(review.Field)].(map[string]any)
	if field["type"] != "string" || field["oneOf"] != nil || field["not"] != nil {
		t.Fatalf("repair field schema permits a non-string removal path: %#v", field)
	}
	_, err = assemblyline.DecodeApplicationJobSpecificationRepair(
		repairInput, fmt.Sprintf(`{%q:null}`, review.Field),
	)
	if err == nil {
		t.Fatal("repair model was allowed to remove a reviewed value")
	}
}

func assertReviewSchemaAllowsRemoval(
	t *testing.T,
	input assemblyline.ApplicationJobSpecificationReviewInput,
) {
	t.Helper()
	schema, err := assemblyline.ApplicationJobSpecificationReviewResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	values := properties["resolution"].(map[string]any)["enum"].([]string)
	if !reflect.DeepEqual(values, []string{"", "replace", "remove"}) {
		t.Fatalf("review resolutions=%#v", values)
	}
}

func currentSpecificationFieldValue(
	t *testing.T,
	value assemblyline.ApplicationJobSpecification,
	field assemblyline.ApplicationJobSpecificationField,
) string {
	t.Helper()
	switch field {
	case assemblyline.ApplicationJobSpecificationRequiredBehaviorsField:
		return value.RequiredBehaviors[0]
	case assemblyline.ApplicationJobSpecificationAcceptanceCriteriaField:
		return value.AcceptanceCriteria[0]
	default:
		t.Fatalf("unsupported fixture field %q", field)
		return ""
	}
}
