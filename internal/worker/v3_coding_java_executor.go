package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func newDirectCodingJavaSourceGenerator(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectSourceGenerator, error) {
	return newDirectCodingLanguageSourceGenerator(session, directCodingLanguageSourceConfig{
		Language: "java", AdapterID: "java",
		ValidateFragment: validateDirectCodingJavaFragment,
	})
}

func validateDirectCodingJavaFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	validated, err := assemblyline.ValidateJavaFragment(input.Signature, candidate)
	if err != nil {
		return "", err
	}
	if err := validateDirectCodingJavaScope(input, validated); err != nil {
		return "", err
	}
	return validated, nil
}
