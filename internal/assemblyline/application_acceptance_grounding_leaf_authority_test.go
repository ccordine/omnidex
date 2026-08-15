package assemblyline

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestApplicationAcceptanceGroundingSemanticRepairsHaveNoLeafCorrectionAuthority(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	job, err := NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		selected map[string]bool
		field    string
	}{
		"unsupported site": {
			selected: map[string]bool{
				"site_002__criterion_001": true,
				"site_003__criterion_002": true,
				"site_004__criterion_002": true,
			},
			field: "site_001__criterion_001",
		},
		"missing criterion": {
			selected: map[string]bool{
				"site_001__criterion_001": true,
				"site_002__criterion_001": true,
				"site_003__criterion_001": true,
				"site_004__criterion_001": true,
			},
			field: "site_001__criterion_002",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			retained := acceptanceGroundingLeafFixtureRaw(t, input, testCase.selected)
			patch, marshalErr := json.Marshal(map[string]bool{testCase.field: true})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, applyErr := ApplyResponseCorrectionForField(
				job, retained, string(patch), testCase.field,
			); applyErr == nil {
				t.Fatal("valid semantic repair was laundered into acceptance")
			}
		})
	}
}

func TestApplicationAcceptanceGroundingRejectsEqualCardinalityExtraBeforeMissingLeaf(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	state := acceptanceGroundingLeafFixture(t, input, nil)
	delete(state, "site_003__criterion_002")
	state["invented_authority"] = true
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	_, decodeErr := DecodeApplicationAcceptanceGroundingReview(input, string(raw))
	if decodeErr == nil {
		t.Fatal("equal-cardinality extra field was accepted")
	}
	var leafErr *ApplicationAcceptanceGroundingLeafValidationError
	if errors.As(decodeErr, &leafErr) {
		t.Fatalf("immutable extra was misclassified as correctable leaf %s: %v", leafErr.Field(), decodeErr)
	}
}

func TestApplicationAcceptanceGroundingNonBooleanLeafHasClosedDefectKind(t *testing.T) {
	t.Parallel()

	input, _ := acceptanceGroundingFixture(t)
	state := make(map[string]any)
	for _, leaf := range input.groundingLeaves() {
		state[leaf.Field] = false
	}
	state["site_001__criterion_001"] = "true"
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	_, decodeErr := DecodeApplicationAcceptanceGroundingReview(input, string(raw))
	var leafErr *ApplicationAcceptanceGroundingLeafValidationError
	if !errors.As(decodeErr, &leafErr) ||
		leafErr.Kind() != ApplicationAcceptanceGroundingLeafNonBoolean {
		t.Fatalf("non-boolean leaf lacks closed defect kind: %v", decodeErr)
	}
}
