package assemblyline

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestApplicationAcceptanceGroundingCriterionLimitCountsRunes(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	input.Criteria[0].Statement = strings.Repeat("界", maxApplicationCriterionRunes)
	if _, err := NewApplicationAcceptanceGroundingReviewJob(input); err != nil {
		t.Fatalf("criterion within rune limit was rejected: %v", err)
	}
	input.Criteria[0].Statement += "界"
	if _, err := NewApplicationAcceptanceGroundingReviewJob(input); err == nil {
		t.Fatal("criterion beyond rune limit was accepted")
	}
}

func TestApplicationAcceptanceGroundingLeafErrorIsBoundedWhileSchemaRetainsSemantics(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	statement := strings.Repeat("界", maxApplicationCriterionRunes)
	input.Criteria[0].Statement = statement
	state := acceptanceGroundingLeafFixture(t, input, nil)
	delete(state, "site_001__criterion_001")
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	_, decodeErr := DecodeApplicationAcceptanceGroundingReview(input, string(raw))
	var leafErr *ApplicationAcceptanceGroundingLeafValidationError
	if !errors.As(decodeErr, &leafErr) || leafErr.Kind() != ApplicationAcceptanceGroundingLeafAbsent {
		t.Fatalf("missing leaf lacks closed failure kind: %v", decodeErr)
	}
	if len(decodeErr.Error()) > 128 || strings.Contains(decodeErr.Error(), "界") {
		t.Fatalf("leaf failure leaked unbounded semantic content: %q", decodeErr)
	}
	schema, err := ApplicationAcceptanceGroundingReviewResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	description := properties[leafErr.Field()].(map[string]any)["description"].(string)
	if !strings.Contains(description, statement) {
		t.Fatal("field-scoped schema omitted exact source-free criterion semantics")
	}
}

func TestApplicationAcceptanceGroundingRejectsForgedInventoryVocabulary(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*AcceptanceObservationSite){
		"private callee": func(site *AcceptanceObservationSite) {
			site.Operations[0] = "harness_call:privateRuntime"
		},
		"identifier structure": func(site *AcceptanceObservationSite) {
			site.Structure = append(site.Structure, "private_identifier")
		},
	} {
		t.Run(name, func(t *testing.T) {
			input, _ := acceptanceGroundingFixture(t)
			mutate(&input.Inventory.Sites[0])
			digest, err := acceptanceObservationInventorySHA(input.Inventory)
			if err != nil {
				t.Fatal(err)
			}
			input.Inventory.InventorySHA256 = digest
			if _, err := NewApplicationAcceptanceGroundingReviewJob(input); err == nil {
				t.Fatal("forged source-bearing inventory vocabulary was accepted")
			}
		})
	}
}

func TestApplicationAcceptanceGroundingRequiresRepairForUntrustedCall(t *testing.T) {
	t.Parallel()

	source := `function VerifyUnknown(): void {
  privateObserver("invented value");
}`
	input, err := NewApplicationAcceptanceGroundingReviewInput(ApplicationTaskContext{
		WorkloadSHA256: strings.Repeat("c", 64),
		Task: ApplicationTaskContextTask{TaskID: "task_003", AcceptanceCriteria: []string{
			"The public status is observable.",
		}},
	}, source, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(input.Inventory.canonicalModelProjection(), "privateObserver") {
		t.Fatal("untrusted private callee leaked into source-free inventory")
	}
	review, err := DecodeApplicationAcceptanceGroundingReview(
		input,
		acceptanceGroundingLeafFixtureRaw(t, input, map[string]bool{
			"site_001__criterion_001": true,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if review.Decision != AcceptanceGroundingRepair || review.UnsupportedSiteID != "site_001" {
		t.Fatalf("untrusted call obtained criterion authority: %+v", review)
	}
	if _, err := AcceptApplicationAcceptanceGroundingReview(input, review); err == nil {
		t.Fatal("derived repair decision produced acceptance receipt")
	}
}

func TestApplicationAcceptanceGroundingPortablePayloadCannotCarrySourceOrAuthorityExpansion(t *testing.T) {
	t.Parallel()

	input, source := acceptanceGroundingFixture(t)
	job, err := NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{source, "privateControl", "file_name", "path", "tools", "implementation"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("portable grounding payload carried forbidden %q: %s", forbidden, encoded)
		}
	}
}
