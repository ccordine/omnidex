package scrum

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestJobMetadataRequiresAndPreservesCodingScopeMode(t *testing.T) {
	metadata := JobMetadata{
		CardID:          "scope-mode-card",
		CardTitle:       "Exercise scope mode metadata",
		ModelConfig:     modelconfig.Config{},
		CodingScopeMode: model.CodingScopeModeStrict,
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeJobMetadata(raw)
	if err != nil {
		t.Fatalf("decode Scrum metadata: %v", err)
	}
	if decoded.CodingScopeMode != model.CodingScopeModeStrict {
		t.Fatalf("Scrum coding scope mode=%q", decoded.CodingScopeMode)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "coding_scope_mode")
	withoutScopeMode, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeJobMetadata(withoutScopeMode); err == nil {
		t.Fatal("expected missing Scrum coding scope mode to fail")
	}
}
