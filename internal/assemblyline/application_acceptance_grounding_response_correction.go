package assemblyline

import (
	"fmt"
	"strings"
)

func buildApplicationAcceptanceGroundingResponseCorrectionPrompt(
	input ResponseCorrectionInput,
) (string, error) {
	originalPrompt, _, err := RenderPortableJob(input.Original)
	if err != nil {
		return "", err
	}
	if input.TargetField == "" {
		return "", fmt.Errorf("acceptance grounding correction requires one target leaf")
	}
	return strings.Join([]string{
		"Return one JSON object containing only the named target leaf.",
		"Resolve the exact validation defect for that leaf from the original semantic question.",
		"TARGET_LEAF: " + input.TargetField,
		"ORIGINAL_SEMANTIC_QUESTION:\n" + originalPrompt,
		"EXACT_VALIDATION_DEFECT:\n" + input.ValidationFailure,
	}, "\n\n"), nil
}
