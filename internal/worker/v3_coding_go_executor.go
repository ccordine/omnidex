package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func validateDirectCodingGoFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	permitted := append([]string(nil), input.Capabilities...)
	permitted = append(permitted, input.PermittedSymbols...)
	validated, err := gofragment.ParseNewFunctionBody(input.Signature, permitted, candidate)
	if err == nil {
		return validated, nil
	}
	var located *gofragment.BodySpanViolation
	if !errors.As(err, &located) {
		return "", err
	}
	if located.StartByte == 0 && located.EndByte == len(candidate) {
		return "", err
	}
	failed := candidate[located.StartByte:located.EndByte]
	replacements, replacementErr := directCodingGoIdentifierChoices(
		input, candidate, failed, located.StartByte,
	)
	if replacementErr != nil {
		return "", fmt.Errorf("enumerate exact Go identifier replacements: %w", replacementErr)
	}
	defect, defectErr := assemblyline.NewSourceBodyIdentifierDefect(
		candidate,
		located.StartByte,
		located.EndByte,
		"Which available value has the meaning required at this unresolved reference?",
		err,
		replacements,
	)
	if defectErr != nil {
		return "", fmt.Errorf("map exact Go identifier to implementation body: %w", defectErr)
	}
	return "", defect
}
