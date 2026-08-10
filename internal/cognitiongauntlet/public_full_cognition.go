package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

// RunPublicFullCognition is the inference-only boundary. Neither its bundle nor
// its request can carry an oracle, witness, generator seed, or evaluator label.
func RunPublicFullCognition(
	ctx context.Context,
	bundle PublicInferenceBundle,
	request PublicFullCognitionRunRequest,
) (PublicFullCognitionRunResult, error) {
	execution, err := preparePublicFullCognition(ctx, bundle, request)
	if err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	run, err := execution.components.runtime.Run(ctx, execution.binding, cognitionruntime.RunLimits{
		MaxCycles: uint32(bundle.Authority.Budget.RuntimeCycles),
	})
	if err != nil {
		return cancelAndFinishPublicCognition(
			ctx, bundle, request, execution, run,
			fmt.Errorf("execute public cognition runtime: %w", err),
		)
	}
	return finishPublicFullCognition(ctx, bundle, request, execution, run)
}

func cancelAndFinishPublicCognition(
	ctx context.Context,
	bundle PublicInferenceBundle,
	request PublicFullCognitionRunRequest,
	execution publicFullCognitionExecution,
	run cognitionruntime.RunResult,
	source error,
) (PublicFullCognitionRunResult, error) {
	if _, registered := classifyRuntimeCancellation(source); !registered {
		return PublicFullCognitionRunResult{}, source
	}
	if err := cancelFullCognitionRuntimeFailure(
		ctx, execution.components, execution.binding, source,
	); err != nil {
		return PublicFullCognitionRunResult{}, err
	}
	return finishPublicFullCognition(ctx, bundle, request, execution, run)
}
