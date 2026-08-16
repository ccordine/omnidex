package architecture

import (
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationJobSpecificationRepairSchemaExcludesNoOpAcrossUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name        string
		product     string
		requirement string
		field       assemblyline.ApplicationJobSpecificationField
		evidenceID  string
		current     string
		retained    assemblyline.ApplicationJobSpecification
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
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			requirement := assemblyline.Requirement{ID: "requirement_001", SourceQuote: fixture.requirement}
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
			review, err := assemblyline.DecodeApplicationJobSpecificationReview(
				reviewInput,
				fmt.Sprintf(`{"decision":"repair","evidence_id":%q,"finding":"The current value is outside the focused requirement."}`, fixture.evidenceID),
			)
			if err != nil {
				t.Fatal(err)
			}
			repairInput, err := assemblyline.NewApplicationJobSpecificationRepairInput(
				authority, fixture.retained, review, 1,
			)
			if err != nil {
				t.Fatal(err)
			}
			schema, err := assemblyline.ApplicationJobSpecificationRepairResponseSchema(repairInput)
			if err != nil {
				t.Fatal(err)
			}
			properties := schema["properties"].(map[string]any)
			fieldSchema := properties[string(fixture.field)].(map[string]any)
			branches := fieldSchema["oneOf"].([]any)
			replacement := branches[0].(map[string]any)
			forbidden := replacement["not"].(map[string]any)
			if forbidden["const"] != fixture.current {
				t.Fatalf("no-op exclusion=%#v want %q", forbidden, fixture.current)
			}
			removal := branches[1].(map[string]any)
			if removal["type"] != "null" {
				t.Fatalf("removal branch=%#v", removal)
			}
		})
	}
}
