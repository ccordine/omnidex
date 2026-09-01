package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func validateDirectCodingJavaFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	validated, err := assemblyline.ValidateJavaFragment(input.Signature, candidate)
	if err != nil {
		return "", err
	}
	if err := validateDirectCodingJavaScope(input, candidate, validated); err != nil {
		return "", directCodingSourceBodyError(input, candidate, validated, err)
	}
	return validated, nil
}
