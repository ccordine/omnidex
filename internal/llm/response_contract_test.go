package llm

import (
	"strings"
	"testing"
)

func TestValidateResponseContractRequiresJSONForSchema(t *testing.T) {
	err := ValidateResponseContract(PreparedModel{
		ResponseSchema: map[string]any{"type": "object"},
	})
	if err == nil || !strings.Contains(err.Error(), `requires response format "json"`) {
		t.Fatalf("error=%v", err)
	}
}

func TestValidateResponseContractRequiresTypedSchema(t *testing.T) {
	err := ValidateResponseContract(PreparedModel{
		ResponseFormat: ResponseFormatJSON,
		ResponseSchema: map[string]any{"properties": map[string]any{}},
	})
	if err == nil || !strings.Contains(err.Error(), "requires a non-empty type") {
		t.Fatalf("error=%v", err)
	}
}

func TestValidateResponseContractAcceptsTypedJSONSchema(t *testing.T) {
	err := ValidateResponseContract(PreparedModel{
		ResponseFormat: ResponseFormatJSON,
		ResponseSchema: map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
