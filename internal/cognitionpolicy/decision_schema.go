package cognitionpolicy

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func decisionSchemaJSON(catalog cognition.ActionCatalog) (json.RawMessage, error) {
	if err := catalog.Validate(); err != nil {
		return nil, fmt.Errorf("%w: action catalog: %v", ErrInvalidConfig, err)
	}
	actionKinds := make([]string, len(catalog.Schemas))
	for index, schema := range catalog.Schemas {
		actionKinds[index] = string(schema.Kind)
	}
	evidence := evidenceRefSchema()
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"obligation_id", "action", "evidence_refs", "expected_effect",
		},
		"properties": map[string]any{
			"obligation_id": boundedStringSchema(128),
			"action": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"kind", "arguments"},
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": actionKinds},
					"arguments": map[string]any{
						"type": "array", "maxItems": cognition.MaxActionArguments,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"name", "value"},
							"properties": map[string]any{
								"name":  boundedStringSchema(128),
								"value": boundedStringSchema(cognition.MaxActionValueBytes),
							},
						},
					},
				},
			},
			"evidence_refs": map[string]any{
				"type": "array", "maxItems": cognition.MaxEvidenceRefs, "items": evidence,
			},
			"expected_effect": boundedStringSchema(cognition.MaxExpectedEffectBytes),
			"ledger_proposals": map[string]any{
				"type": "array", "maxItems": cognition.MaxLedgerProposals,
				"items": ledgerProposalSchema(evidence),
			},
			"attention_requests": map[string]any{
				"type": "array", "maxItems": cognition.MaxAttentionRequests,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"operation", "target_ref", "scope", "reason"},
					"properties": map[string]any{
						"operation":  map[string]any{"type": "string", "enum": []string{"retain", "release"}},
						"target_ref": evidence,
						"scope": map[string]any{
							"type": "string", "enum": []string{"decision", "obligation", "episode"},
						},
						"reason": boundedStringSchema(cognition.MaxAttentionReasonBytes),
					},
				},
			},
		},
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("%w: render decision schema: %v", ErrInvalidConfig, err)
	}
	return raw, nil
}

func evidenceRefSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"observation_id", "revision", "sha256"},
		"properties": map[string]any{
			"observation_id": boundedStringSchema(128),
			"revision": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"episode_id", "number", "sha256"},
				"properties": map[string]any{
					"episode_id": boundedStringSchema(128),
					"number":     map[string]any{"type": "integer", "minimum": 1},
					"sha256":     digestStringSchema(),
				},
			},
			"sha256": digestStringSchema(),
		},
	}
}

func boundedStringSchema(maximum int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": maximum}
}

func digestStringSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"}
}
