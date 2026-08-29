package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestWebStationWireContractsContainNoExecutionAuthority(t *testing.T) {
	values := []any{
		WebRelevanceInput{}, WebRelevanceCandidate{}, WebRelevanceDecision{},
		WebRelevanceRelationInput{}, WebRelevanceRelationDecision{},
		WebGroundedSynthesisInput{}, WebGroundedEvidence{},
		WebGroundedSynthesisDecision{}, WebGroundedParagraph{},
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
