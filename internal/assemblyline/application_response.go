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

func RepositoryRequirementInterpretationResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "requirements"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": RepositoryRequirementInterpretationSchemaV2,
			},
			"requirements": map[string]any{
				"type": "array", "minItems": 1, "maxItems": maxRequirementCount,
				"items": map[string]any{
					"type": "string", "minLength": 1, "maxLength": maxRequirementQuoteBytes,
				},
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
