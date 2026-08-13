package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestWebStationWireContractsContainNoExecutionAuthority(t *testing.T) {
	values := []any{
		WebSearchTermsInput{}, WebSearchTermsDecision{},
		WebRelevanceInput{}, WebRelevanceCandidate{}, WebRelevanceDecision{},
		WebGroundedSynthesisInput{}, WebGroundedEvidence{},
		WebGroundedSynthesisDecision{}, WebGroundedParagraph{},
		WebGroundedSynthesisCorrectionInput{}, WebGroundedSynthesisCorrectionDecision{},
		WebClaimEvidenceReviewInput{}, WebReviewParagraph{}, WebReviewEvidence{},
		WebClaimEvidenceReviewDecision{},
	}
	for _, value := range values {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			wire := strings.Split(field.Tag.Get("json"), ",")[0]
			forbidden := map[string]struct{}{
				"tool": {}, "action": {}, "operation": {}, "path": {}, "plan": {},
			}
			if _, exists := forbidden[wire]; exists {
				t.Fatalf("%s exposes forbidden field %q", typeOf.Name(), wire)
			}
		}
	}
}

func TestWebSynthesisCorrectionDecisionContainsExactlyOneTextLeaf(t *testing.T) {
	typeOf := reflect.TypeOf(WebGroundedSynthesisCorrectionDecision{})
	if typeOf.NumField() != 1 || typeOf.Field(0).Name != "Text" ||
		strings.Split(typeOf.Field(0).Tag.Get("json"), ",")[0] != "text" {
		t.Fatalf("web synthesis correction may return only one text leaf: %v", typeOf)
	}
}
