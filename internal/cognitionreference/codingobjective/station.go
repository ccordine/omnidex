package codingobjective

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func generateDeclaration(
	ctx context.Context,
	station DeclarationStation,
	repository inspectedRepository,
	result *Result,
) (string, error) {
	input := fragmentInput(repository)
	job, err := assemblyline.NewFragmentModificationJob(input)
	if err != nil {
		return "", fmt.Errorf("build bounded declaration job: %w", err)
	}
	frozen := clonePortableJob(job)
	dispatched := clonePortableJob(frozen)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result.PortableJobID = frozen.ID
	result.ModelCalls++
	result.Steps = append(result.Steps, StepDeclarationDispatched)
	response, callErr := station.Generate(ctx, dispatched)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !samePortableJob(dispatched, frozen) {
		return "", fmt.Errorf("%w: declaration station mutated its immutable job", ErrDeclaration)
	}
	if callErr != nil {
		return "", fmt.Errorf("declaration station: %w", callErr)
	}
	if err := response.ValidateFor(frozen); err != nil {
		return "", fmt.Errorf("%w: %v", ErrDeclaration, err)
	}
	contract := gofragment.Contract{
		Signature: input.Signature, Current: input.CurrentDeclaration,
		PermittedSymbols: append([]string{}, input.PermittedSymbols...),
	}
	current, err := gofragment.ParseFunction(contract, input.CurrentDeclaration)
	if err != nil {
		return "", fmt.Errorf("validate current target declaration: %w", err)
	}
	candidate, err := gofragment.ParseFunction(contract, response.Candidate)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDeclaration, err)
	}
	if candidate == current {
		return "", fmt.Errorf("%w: unchanged declaration", ErrDeclaration)
	}
	result.Steps = append(result.Steps, StepDeclarationAccepted)
	return candidate, nil
}

func clonePortableJob(job assemblyline.PortableJob) assemblyline.PortableJob {
	job.Payload = bytes.Clone(job.Payload)
	return job
}

func samePortableJob(left, right assemblyline.PortableJob) bool {
	return left.Schema == right.Schema && left.ID == right.ID && left.Kind == right.Kind &&
		bytes.Equal(left.Payload, right.Payload)
}
