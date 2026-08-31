package worker

import (
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func validateDirectCodingGoFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	permitted := append([]string(nil), input.Capabilities...)
	permitted = append(permitted, input.PermittedSymbols...)
	return gofragment.ParseNewFunction(input.Signature, permitted, candidate)
}
