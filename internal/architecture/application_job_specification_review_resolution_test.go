package architecture

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestSpecificationReviewCandidatesAreAppliedByCodeAcrossUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name        string
		product     string
		requirement string
		field       assemblyline.ApplicationJobSpecificationField
		evidenceID  string
		retained    assemblyline.ApplicationJobSpecification
		replacement string
		wantReplace assemblyline.ApplicationJobSpecification
		wantRemove  assemblyline.ApplicationJobSpecification
	}{
		{
			name: "inventory behavior", product: "inventory console",
			requirement: "Users filter visible inventory by status.",
			field:       assemblyline.ApplicationJobSpecificationRequiredBehaviorsField,
			evidenceID:  "E2",
			retained: assemblyline.ApplicationJobSpecification{
				Objective: "Implement status filtering.",
				RequiredBehaviors: []string{
					"Users select a status and see matching inventory.",
					"Users export a visible inventory report.",
				},
				AcceptanceCriteria: []string{"Selecting a status excludes other statuses."},
			},
			replacement: "Users apply a status filter and see matching inventory.",
			wantReplace: assemblyline.ApplicationJobSpecification{
				Objective: "Implement status filtering.",
				RequiredBehaviors: []string{
					"Users select a status and see matching inventory.",
					"Users apply a status filter and see matching inventory.",
				},
				AcceptanceCriteria: []string{"Selecting a status excludes other statuses."},
			},
			wantRemove: assemblyline.ApplicationJobSpecification{
				Objective:          "Implement status filtering.",
				RequiredBehaviors:  []string{"Users select a status and see matching inventory."},
				AcceptanceCriteria: []string{"Selecting a status excludes other statuses."},
			},
		},
		{
			name: "appointment criterion", product: "appointment portal",
			requirement: "Users dismiss one visible appointment reminder.",
			field:       assemblyline.ApplicationJobSpecificationAcceptanceCriteriaField,
			evidenceID:  "E2",
			retained: assemblyline.ApplicationJobSpecification{
				Objective:         "Implement reminder dismissal.",
				RequiredBehaviors: []string{"Users dismiss one visible reminder."},
				AcceptanceCriteria: []string{
					"The dismissed reminder is no longer visible.",
					"The appointment can be rescheduled.",
				},
			},
			replacement: "The dismissed reminder remains absent after the page refreshes.",
			wantReplace: assemblyline.ApplicationJobSpecification{
				Objective:         "Implement reminder dismissal.",
				RequiredBehaviors: []string{"Users dismiss one visible reminder."},
				AcceptanceCriteria: []string{
					"The dismissed reminder is no longer visible.",
					"The dismissed reminder remains absent after the page refreshes.",
				},
			},
			wantRemove: assemblyline.ApplicationJobSpecification{
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
			authority := specificationFixtureAuthority(fixture.product, fixture.requirement)
			input, err := assemblyline.NewApplicationJobSpecificationReviewInput(
				authority, fixture.retained, fixture.field, fixture.evidenceID, 1,
			)
			if err != nil {
				t.Fatal(err)
			}
			assertReviewSchema(t, input)
			accepted, err := assemblyline.DecodeApplicationJobSpecificationReview(
				input, fmt.Sprintf(
					`{"decision":"accept","evidence_id":%q,"replacement_value":"unused"}`,
					fixture.evidenceID,
				),
			)
			if err != nil || accepted.Decision != assemblyline.ApplicationJobSpecificationReviewAccept {
				t.Fatalf("acceptance with untrusted non-mutating fields rejected: review=%#v err=%v", accepted, err)
			}

			replace, err := assemblyline.DecodeApplicationJobSpecificationReview(
				input, fmt.Sprintf(
					`{"decision":"replace","evidence_id":%q,"replacement_value":%q}`,
					fixture.evidenceID, fixture.replacement,
				),
			)
			if err != nil {
				t.Fatal(err)
			}
			updated, err := assemblyline.ApplyApplicationJobSpecificationReviewReplacement(
				authority, fixture.retained, replace,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(updated, fixture.wantReplace) {
				t.Fatalf("replacement updated=%#v want %#v", updated, fixture.wantReplace)
			}

			remove, err := assemblyline.DecodeApplicationJobSpecificationReview(
				input, fmt.Sprintf(
					`{"decision":"remove","evidence_id":%q,"replacement_value":""}`,
					fixture.evidenceID,
				),
			)
			if err != nil {
				t.Fatal(err)
			}
			updated, err = assemblyline.ApplyApplicationJobSpecificationReviewRemoval(
				authority, fixture.retained, remove,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(updated, fixture.wantRemove) {
				t.Fatalf("removal updated=%#v want %#v", updated, fixture.wantRemove)
			}

			finalInput, err := assemblyline.NewApplicationJobSpecificationReviewInput(
				authority, updated, fixture.field, "E1", 2,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = assemblyline.DecodeApplicationJobSpecificationReview(
				finalInput,
				`{"decision":"remove","evidence_id":"E1","replacement_value":""}`,
			)
			if err == nil {
				t.Fatal("removing the final required list value was accepted")
			}
		})
	}
}

func specificationFixtureAuthority(product, requirement string) assemblyline.ApplicationJobSpecificationInput {
	focused := assemblyline.Requirement{ID: "requirement_001", SourceQuote: requirement}
	return assemblyline.ApplicationJobSpecificationInput{
		Surface:              assemblyline.ApplicationSurfaceBrowser,
		ProductQuote:         product,
		AcceptedRequirements: []assemblyline.Requirement{focused},
		FocusedRequirement:   focused,
	}
}

func assertReviewSchema(t *testing.T, input assemblyline.ApplicationJobSpecificationReviewInput) {
	t.Helper()
	schema, err := assemblyline.ApplicationJobSpecificationReviewResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	decisions := properties["decision"].(map[string]any)["enum"].([]string)
	if !reflect.DeepEqual(decisions, []string{"accept", "replace", "remove"}) {
		t.Fatalf("review decisions=%#v", decisions)
	}
	if _, exists := properties["finding"]; exists {
		t.Fatal("review schema retained a prose finding field")
	}
	replacement := properties["replacement_value"].(map[string]any)
	if replacement["type"] != "string" {
		t.Fatalf("replacement schema=%#v", replacement)
	}
}

func TestIntentReviewCandidatesAreAppliedByCodeAcrossUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name         string
		request      string
		product      string
		requirements []string
		target       string
		replacement  string
	}{
		{
			name: "inventory obligation", request: "Build an inventory console that filters stock by status.",
			product: "An inventory console", requirements: []string{"Show visible inventory.", "Filter inventory by status."},
			target: "requirements_002", replacement: "Allow users to filter visible inventory by status.",
		},
		{
			name: "appointment obligation", request: "Build an appointment portal where users can dismiss reminders.",
			product: "An appointment portal", requirements: []string{"Show appointment reminders.", "Dismiss a visible reminder."},
			target: "requirements_002", replacement: "Allow users to dismiss one visible appointment reminder.",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			context, err := assemblyline.BootstrapApplicationContext(fixture.request, assemblyline.ApplicationWorkspaceEmpty, nil)
			if err != nil {
				t.Fatal(err)
			}
			authority := assemblyline.ApplicationIntentInput{UserRequest: fixture.request, Context: context}
			retained := assemblyline.ApplicationIntentCandidate{
				Schema: assemblyline.ApplicationIntentCandidateSchemaV1, ProductContext: fixture.product,
				Requirements: append([]string(nil), fixture.requirements...),
			}
			input := assemblyline.ApplicationIntentReviewInput{Authority: authority, Candidate: retained, Target: fixture.target}
			job, err := assemblyline.NewApplicationIntentReviewJob(input)
			if err != nil {
				t.Fatal(err)
			}
			_, schema, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			properties := schema["properties"].(map[string]any)
			if _, exists := properties["finding"]; exists {
				t.Fatal("review schema exposes a prose finding")
			}
			if properties["replacement_value"] == nil {
				t.Fatal("review schema omits direct candidate value")
			}

			review, err := assemblyline.DecodeApplicationIntentReview(input, fmt.Sprintf(
				`{"schema":"omnidex.application-intent-review.v1","decision":"replace","replacement_value":%q}`,
				fixture.replacement,
			))
			if err != nil {
				t.Fatal(err)
			}
			updated, err := assemblyline.ApplyApplicationIntentReviewReplacement(authority, retained, review)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Requirements[0] != fixture.requirements[0] || updated.Requirements[1] != fixture.replacement {
				t.Fatalf("code did not splice only the selected leaf: %#v", updated)
			}
			_, err = assemblyline.ApplyApplicationIntentReviewReplacement(authority, updated, review)
			if err == nil {
				t.Fatal("stale review was applied to changed retained state")
			}
		})
	}
}
