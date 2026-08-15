package assemblyline

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestApplicationAcceptanceGroundingLeafWireDerivesAcceptedReview(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	selected := map[string]bool{
		"site_001__criterion_001": true,
		"site_002__criterion_001": true,
		"site_003__criterion_002": true,
		"site_004__criterion_002": true,
	}
	raw := acceptanceGroundingLeafFixtureRaw(t, input, selected)
	review, err := DecodeApplicationAcceptanceGroundingReview(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if review.Decision != AcceptanceGroundingAccept || review.UnsupportedSiteID != "" ||
		review.MissingCriterionID != "" || len(review.Mappings) != 4 {
		t.Fatalf("review was not derived from complete leaves: %+v", review)
	}
	want := []AcceptanceGroundingMapping{
		{SiteID: "site_001", AuthorityIDs: []string{"criterion_001"}},
		{SiteID: "site_002", AuthorityIDs: []string{"criterion_001"}},
		{SiteID: "site_003", AuthorityIDs: []string{"criterion_002"}},
		{SiteID: "site_004", AuthorityIDs: []string{"criterion_002"}},
	}
	if !reflect.DeepEqual(review.Mappings, want) {
		t.Fatalf("mappings=%+v want=%+v", review.Mappings, want)
	}

	schema, err := ApplicationAcceptanceGroundingReviewResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 8 || schema["oneOf"] != nil {
		t.Fatalf("response is not one fixed leaf-addressable object: %#v", schema)
	}
	for field, definition := range properties {
		fields, ok := definition.(map[string]any)
		if !ok || fields["type"] != "boolean" || strings.TrimSpace(fields["description"].(string)) == "" {
			t.Fatalf("leaf %s lacks boolean source-free semantics: %#v", field, definition)
		}
	}
	prompt, err := BuildApplicationAcceptanceGroundingReviewPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "site_001__criterion_001") ||
		strings.Contains(prompt, `"decision"`) || strings.Contains(prompt, `"mappings"`) {
		t.Fatalf("prompt does not describe the fixed leaf wire: %s", prompt)
	}
}

func TestApplicationAcceptanceGroundingLeafWireDerivesTypedRepairs(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	unsupported, err := DecodeApplicationAcceptanceGroundingReview(
		input,
		acceptanceGroundingLeafFixtureRaw(t, input, map[string]bool{
			"site_002__criterion_001": true,
			"site_003__criterion_002": true,
			"site_004__criterion_002": true,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Decision != AcceptanceGroundingRepair || unsupported.UnsupportedSiteID != "site_001" {
		t.Fatalf("unsupported site was not code-derived: %+v", unsupported)
	}

	missing, err := DecodeApplicationAcceptanceGroundingReview(
		input,
		acceptanceGroundingLeafFixtureRaw(t, input, map[string]bool{
			"site_001__criterion_001": true,
			"site_002__criterion_001": true,
			"site_003__criterion_001": true,
			"site_004__criterion_001": true,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Decision != AcceptanceGroundingRepair || missing.MissingCriterionID != "criterion_002" {
		t.Fatalf("missing criterion was not code-derived: %+v", missing)
	}
}

func TestApplicationAcceptanceGroundingLeafWireSupportsProgressiveOneLeafCorrection(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	job, err := NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := NewResponseCorrectionJobForField(
		job, "one exact grounding leaf is absent", "site_001__criterion_001",
	)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(correction)
	if err != nil {
		t.Fatal(err)
	}
	if schema["minProperties"] != 1 || schema["maxProperties"] != 1 ||
		strings.Contains(prompt, `"site_001__criterion_001":false`) {
		t.Fatalf("correction exposed retained state or full-state authority: prompt=%s schema=%#v", prompt, schema)
	}

	retained := `{}`
	selected := map[string]bool{
		"site_001__criterion_001": true,
		"site_002__criterion_001": true,
		"site_003__criterion_002": true,
		"site_004__criterion_002": true,
	}
	leaves := input.groundingLeaves()
	for index, leaf := range leaves {
		patchBytes, marshalErr := json.Marshal(map[string]bool{leaf.Field: selected[leaf.Field]})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		retained, err = ApplyResponseCorrectionForField(
			job, retained, string(patchBytes), leaf.Field,
		)
		if err != nil {
			t.Fatalf("progressive leaf %d: %v", index+1, err)
		}
		review, decodeErr := DecodeApplicationAcceptanceGroundingReview(input, retained)
		if index < len(leaves)-1 && decodeErr == nil {
			t.Fatalf("progressive leaf %d prematurely completed: %+v", index+1, review)
		}
		if index == len(leaves)-1 && (decodeErr != nil || review.Decision != AcceptanceGroundingAccept) {
			t.Fatalf("%d distinct structural leaf repairs did not converge: review=%+v error=%v", len(leaves), review, decodeErr)
		}
	}
	if _, err := ApplyResponseCorrectionForField(
		job, retained, retained, "site_001__criterion_001",
	); err == nil {
		t.Fatal("correction was allowed to reconstruct the complete retained candidate")
	}
}

func TestApplicationAcceptanceGroundingLeafWireCanAddOneMissingKnownLeaf(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	job, err := NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	complete := acceptanceGroundingLeafFixture(t, input, map[string]bool{
		"site_001__criterion_001": true,
		"site_002__criterion_001": true,
		"site_003__criterion_002": true,
		"site_004__criterion_002": true,
	})
	delete(complete, "site_004__criterion_002")
	raw, err := json.Marshal(complete)
	if err != nil {
		t.Fatal(err)
	}
	_, decodeErr := DecodeApplicationAcceptanceGroundingReview(input, string(raw))
	if decodeErr == nil {
		t.Fatal("incomplete initial leaf state was accepted")
	}
	var leafErr *ApplicationAcceptanceGroundingLeafValidationError
	if !errors.As(decodeErr, &leafErr) || leafErr.Field() != "site_004__criterion_002" ||
		leafErr.Kind() != ApplicationAcceptanceGroundingLeafAbsent {
		t.Fatalf("missing leaf lacks typed correction identity: %v", decodeErr)
	}
	corrected, err := ApplyResponseCorrectionForField(
		job, string(raw), `{"site_004__criterion_002":true}`, leafErr.Field(),
	)
	if err != nil {
		t.Fatal(err)
	}
	review, err := DecodeApplicationAcceptanceGroundingReview(input, corrected)
	if err != nil || review.Decision != AcceptanceGroundingAccept {
		t.Fatalf("one known absent leaf was not recoverable: review=%+v error=%v", review, err)
	}
}

func TestApplicationAcceptanceGroundingLeafCorrectionRejectsUnrelatedProgress(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	job, err := NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	retained := acceptanceGroundingLeafFixture(t, input, nil)
	delete(retained, "site_003__criterion_002")
	raw, err := json.Marshal(retained)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyResponseCorrectionForField(
		job, string(raw), `{"site_001__criterion_001":true}`,
		"site_003__criterion_002",
	); err == nil {
		t.Fatal("unrelated leaf was accepted for one exact missing-leaf defect")
	}
	correction, err := NewResponseCorrectionJobForField(
		job, "one exact grounding leaf is absent", "site_003__criterion_002",
	)
	if err != nil {
		t.Fatal(err)
	}
	_, schema, err := RenderPortableJob(correction)
	if err != nil {
		t.Fatal(err)
	}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) != 1 || properties["site_003__criterion_002"] == nil {
		t.Fatalf("correction schema exposed unrelated retained leaves: %#v", schema)
	}
}

func TestApplicationAcceptanceGroundingLeafWireRejectsLegacyOrInexactResponses(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	for name, raw := range map[string]string{
		"legacy accept array": `{"decision":"accept","mappings":[]}`,
		"legacy repair union": `{"decision":"repair","unsupported_site_id":"site_001"}`,
		"extra field":         acceptanceGroundingLeafFixtureRaw(t, input, map[string]bool{"extra": true}),
		"markdown":            "```json\n" + acceptanceGroundingLeafFixtureRaw(t, input, nil) + "\n```",
		"trailing prose":      acceptanceGroundingLeafFixtureRaw(t, input, nil) + " accepted",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationAcceptanceGroundingReview(input, raw); err == nil {
				t.Fatal("inexact or retired grounding wire was accepted")
			}
		})
	}
	oversized := strings.Repeat(" ", maxPortableCandidateBytes+1) +
		acceptanceGroundingLeafFixtureRaw(t, input, map[string]bool{
			"site_001__criterion_001": true,
			"site_002__criterion_001": true,
			"site_003__criterion_002": true,
			"site_004__criterion_002": true,
		})
	if _, err := DecodeApplicationAcceptanceGroundingReview(input, oversized); err != nil {
		t.Fatalf("output length alone rejected a valid fixed state: %v", err)
	}
}

func acceptanceGroundingLeafFixtureRaw(
	t *testing.T,
	input ApplicationAcceptanceGroundingReviewInput,
	selected map[string]bool,
) string {
	t.Helper()
	raw, err := json.Marshal(acceptanceGroundingLeafFixture(t, input, selected))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func acceptanceGroundingLeafFixture(
	t *testing.T,
	input ApplicationAcceptanceGroundingReviewInput,
	selected map[string]bool,
) map[string]bool {
	t.Helper()
	result := make(map[string]bool)
	for _, site := range input.reviewSites() {
		for _, criterion := range input.Criteria {
			field := acceptanceGroundingLeafField(site.ID, criterion.ID)
			result[field] = selected[field]
		}
	}
	for field, value := range selected {
		if _, exists := result[field]; !exists {
			result[field] = value
		}
	}
	return result
}
