package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

// executeInitialFragmentGenerationWithReplacement preserves an unresolved
// source leaf across exactly one provider output-limit failure. The rejected
// response bytes are unavailable here by construction and never enter the
// replacement payload or prompt.
func executeInitialFragmentGenerationWithReplacement(
	runtime typedWorkerRuntime,
	initial assemblyline.PortableJob,
	input assemblyline.FragmentGenerationInput,
	modelName string,
) (assemblyline.PortableJob, assemblyline.PortableResult, error) {
	if initial.Kind != assemblyline.WorkFragmentGeneration {
		return initial, assemblyline.PortableResult{}, fmt.Errorf(
			"fragment generation replacement requires one initial generation job",
		)
	}
	expected, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		return initial, assemblyline.PortableResult{}, err
	}
	if expected.ID != initial.ID {
		return initial, assemblyline.PortableResult{}, fmt.Errorf(
			"fragment generation replacement input differs from its initial work",
		)
	}
	result, err := runtime.Execute(initial, modelName)
	if err == nil {
		return initial, result, nil
	}
	var outputLimit *persistedFragmentGenerationOutputLimitFailure
	if !errors.As(err, &outputLimit) {
		return initial, assemblyline.PortableResult{}, err
	}
	if validationErr := outputLimit.Validate(); validationErr != nil {
		return initial, assemblyline.PortableResult{}, errors.Join(err, validationErr)
	}
	replacement, replacementErr := assemblyline.NewFragmentGenerationReplacementJob(
		assemblyline.FragmentGenerationReplacementInput{
			Original: input,
		},
	)
	if replacementErr != nil {
		return initial, assemblyline.PortableResult{}, errors.Join(err, replacementErr)
	}
	if runtime.ExecuteFragmentGenerationReplacement == nil {
		return initial, assemblyline.PortableResult{}, fmt.Errorf(
			"fragment generation replacement requires exact persisted origin execution authority",
		)
	}
	replacementResult, replacementErr := runtime.ExecuteFragmentGenerationReplacement(
		replacement, modelName, queue.StationGapReplacementOrigin{
			GapOpeningID:  outputLimit.OriginGapOpeningID,
			CallReceiptID: outputLimit.OriginCallReceiptID,
		},
	)
	if replacementErr != nil {
		return replacement, assemblyline.PortableResult{}, fmt.Errorf(
			"bounded fragment generation replacement failed after exact output-limit evidence: %w",
			replacementErr,
		)
	}
	return replacement, replacementResult, nil
}
