package cognitionpolicy

import "github.com/gryph/omnidex/internal/cognition"

func ledgerProposalSchema(evidence map[string]any) map[string]any {
	return map[string]any{
		"oneOf": []any{
			textProposalSchema("observation", evidence, true),
			textProposalSchema("hypothesis", evidence, true),
			textProposalSchema("question", evidence, false),
			obligationProposalSchema(evidence),
			revisionProposalSchema(evidence),
			planRevisionProposalSchema(evidence),
		},
	}
}

func planRevisionProposalSchema(evidence map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind", "plan_revision"},
		"properties": map[string]any{
			"kind": map[string]any{"const": "plan_revision"},
			"plan_revision": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"next", "evidence_refs"},
				"properties": map[string]any{
					"next":          goalExpressionSchema(),
					"evidence_refs": boundedEvidenceArraySchema(evidence, 1),
				},
			},
		},
	}
}

func revisionProposalSchema(evidence map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind", "revision"},
		"properties": map[string]any{
			"kind": map[string]any{"const": "revision"},
			"revision": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"target_ref", "evidence_refs"},
				"properties": map[string]any{
					"target_ref": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"uri", "version", "content_sha256"},
						"properties": map[string]any{
							"uri":            boundedStringSchema(cognition.MaxEpistemicRefURIBytes),
							"version":        boundedStringSchema(cognition.MaxVersionBytes),
							"content_sha256": digestStringSchema(),
						},
					},
					"evidence_refs": boundedEvidenceArraySchema(evidence, 1),
				},
			},
		},
	}
}

func textProposalSchema(
	kind string,
	evidence map[string]any,
	requireEvidence bool,
) map[string]any {
	required := []string{"kind", "content"}
	minimum := 0
	if requireEvidence {
		required = append(required, "evidence_refs")
		minimum = 1
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties": map[string]any{
			"kind":          map[string]any{"const": kind},
			"content":       boundedStringSchema(cognition.MaxProposalBytes),
			"evidence_refs": boundedEvidenceArraySchema(evidence, minimum),
		},
	}
}

func obligationProposalSchema(evidence map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind", "obligation"},
		"properties": map[string]any{
			"kind": map[string]any{"const": "obligation"},
			"obligation": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"desired", "evidence_refs"},
				"properties": map[string]any{
					"desired":       goalExpressionSchema(),
					"evidence_refs": boundedEvidenceArraySchema(evidence, 1),
				},
			},
		},
	}
}

func goalExpressionSchema() map[string]any {
	predicates := func(minimum int) map[string]any {
		return map[string]any{
			"type": "array", "minItems": minimum,
			"maxItems": cognition.MaxGoalPredicates, "items": predicateSchema(),
		}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"all": predicates(0), "any": predicates(0), "not": predicates(0),
		},
		"anyOf": []any{
			map[string]any{"required": []string{"all"}, "properties": map[string]any{"all": predicates(1)}},
			map[string]any{"required": []string{"any"}, "properties": map[string]any{"any": predicates(1)}},
			map[string]any{"required": []string{"not"}, "properties": map[string]any{"not": predicates(1)}},
		},
	}
}

func predicateSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"name", "args"},
		"properties": map[string]any{
			"name": boundedStringSchema(cognition.MaxIdentityBytes),
			"args": map[string]any{
				"type": "array", "maxItems": cognition.MaxPredicateArgs,
				"items": boundedStringSchema(cognition.MaxPredicateArgBytes),
			},
		},
	}
}

func boundedEvidenceArraySchema(evidence map[string]any, minimum int) map[string]any {
	return map[string]any{
		"type": "array", "minItems": minimum,
		"maxItems": cognition.MaxEvidenceRefs, "items": evidence,
	}
}
