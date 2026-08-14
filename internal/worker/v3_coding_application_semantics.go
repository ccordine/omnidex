package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runDirectCodingApplicationInterpreter(
	runtime typedWorkerRuntime,
	requirementModel string,
	surfaceModel string,
	artifactModel string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationSpecification, error) {
	var zero assemblyline.ApplicationSpecification

	classification, err := classifyApplicationSurface(runtime, surfaceModel, authority, identities)
	if err != nil {
		return zero, err
	}
	if classification.Surface != assemblyline.ApplicationSurfaceBrowser {
		return zero, fmt.Errorf(
			"generic coding runtime does not yet support application surface %s",
			classification.Surface,
		)
	}
	resolution, err := interpretApplicationRequirements(
		runtime, requirementModel, authority, identities,
	)
	if err != nil {
		return zero, err
	}
	artifacts, err := classifyArtifactHandling(runtime, artifactModel, authority, identities)
	if err != nil {
		return zero, err
	}
	specification := assemblyline.ApplicationSpecification{
		Surface: classification.Surface, ProductQuote: resolution.ProductQuote,
		Requirements: resolution.Requirements, Artifacts: artifacts,
	}
	if err := specification.Validate(); err != nil {
		return zero, err
	}
	return specification, nil
}

func classifyApplicationSurface(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationClassification, error) {
	jobInput := assemblyline.ApplicationClassificationInput{UserRequest: authority}
	job, err := assemblyline.NewApplicationClassificationJob(jobInput)
	if err != nil {
		return assemblyline.ApplicationClassification{}, err
	}
	return runDirectCodingSemanticCall[assemblyline.ApplicationClassification](
		runtime, modelName, "application_surface", job, identities,
		func(value assemblyline.ApplicationClassification) error { return value.Validate() },
	)
}

func interpretApplicationRequirements(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementResolution, error) {
	var zero assemblyline.ApplicationRequirementResolution
	input := assemblyline.ApplicationRequirementInterpretationInput{UserRequest: authority}
	job, err := assemblyline.NewApplicationRequirementInterpretationJob(input)
	if err != nil {
		return zero, err
	}
	oneCallRuntime := runtime
	oneCallRuntime.MaxAttempts = 1
	interpretation, err := runDirectCodingSemanticCall[assemblyline.ApplicationRequirementInterpretation](
		oneCallRuntime, modelName, "application_requirements", job, identities,
		func(value assemblyline.ApplicationRequirementInterpretation) error {
			_, validationErr := assemblyline.ResolveApplicationRequirements(input, value)
			return validationErr
		},
	)
	if err != nil {
		return zero, err
	}
	return assemblyline.ResolveApplicationRequirements(input, interpretation)
}
