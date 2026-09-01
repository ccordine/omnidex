package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/gofragment"
)

// ExtractFragmentGenerationSourceBody deterministically projects one gross
// ordinary provider response to the implementation body retained by code. Raw
// response bytes remain evidence; only this extracted body can become mutable
// source state for validation and bounded correction.
func ExtractFragmentGenerationSourceBody(job PortableJob, raw string) (string, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	if job.Kind != WorkFragmentGeneration {
		return "", fmt.Errorf("portable work kind %q has no source-body extraction", job.Kind)
	}
	var input FragmentGenerationInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return "", err
	}
	switch input.Language {
	case "go":
		return gofragment.ExtractNewFunctionBodyResponse(input.Signature, raw)
	case "typescript":
		// TSX parses ordinary TypeScript declarations as a subset while also
		// retaining JSX bodies. The exact document parser still validates the
		// extracted body later under its code-owned TS/TSX authority.
		return ExtractTypeScriptFunctionBodyResponse(
			TypeScriptFunctionContract{Signature: input.Signature, TSX: true}, raw,
		)
	case "javascript":
		return ExtractJavaScriptSourceBodyResponse(input.Signature, raw)
	case "java":
		return ExtractJavaSourceBodyResponse(input.Signature, raw)
	case "rust":
		return ExtractRustSourceBodyResponse(input.Signature, raw)
	case TextFragmentLanguage:
		return NormalizeTextFragmentResponse(raw)
	default:
		return "", fmt.Errorf(
			"fragment generation language %q has no source-body extractor", input.Language,
		)
	}
}
