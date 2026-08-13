package assemblyline

import "fmt"

func ApplicationClassificationResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "surface"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": ApplicationClassificationSchemaV1},
			"surface": enumSchema(
				ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine,
				ApplicationSurfaceService, ApplicationSurfaceUnsupported,
			),
		},
	)
}

func ApplicationIdentityResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "product_quote"},
		map[string]any{
			"schema":        map[string]any{"type": "string", "const": ApplicationIdentitySchemaV1},
			"product_quote": map[string]any{"type": "string", "maxLength": maxApplicationProductBytes},
		},
	)
}

func RequirementPartitionResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "feature_quotes"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": RequirementPartitionSchemaV1},
			"feature_quotes": map[string]any{
				"type": "array", "minItems": 0, "maxItems": maxRequirementPartitionCount,
				"items": map[string]any{"type": "string", "maxLength": maxRequirementQuoteBytes},
			},
		},
	)
}

func (decision ArtifactHandlingDecision) Validate(token string) error {
	if decision.Schema != ArtifactHandlingSchemaV1 {
		return fmt.Errorf("artifact handling schema must be %q", ArtifactHandlingSchemaV1)
	}
	if decision.Token != token {
		return fmt.Errorf("artifact handling token %q does not match focused token %q", decision.Token, token)
	}
	switch decision.Handling {
	case ArtifactPreserveUnchanged, ArtifactMustExist, ArtifactMustBeAbsent,
		ArtifactPossibleAbsenceCandidate, ArtifactMentionedOnly:
		return nil
	default:
		return fmt.Errorf("artifact handling %q is unsupported", decision.Handling)
	}
}

func ArtifactHandlingResponseSchema(token string) map[string]any {
	return objectSchema(
		[]string{"schema", "token", "handling"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": ArtifactHandlingSchemaV1},
			"token":  map[string]any{"type": "string", "const": token},
			"handling": enumSchema(
				ArtifactPreserveUnchanged,
				ArtifactMustExist,
				ArtifactMustBeAbsent,
				ArtifactPossibleAbsenceCandidate,
				ArtifactMentionedOnly,
			),
		},
	)
}
