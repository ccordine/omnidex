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

func TestValidateResponseContractRejectsStructuredNativeThinking(t *testing.T) {
	for _, prepared := range []PreparedModel{
		{ThinkingEnabled: true, ResponseFormat: ResponseFormatJSON},
		{ThinkingEnabled: true, ResponseSchema: map[string]any{"type": "object"}},
	} {
		if err := ValidateResponseContract(prepared); err == nil || !strings.Contains(err.Error(), "native thinking forbids") {
			t.Fatalf("ValidateResponseContract(%#v) error=%v", prepared, err)
		}
	}
}
