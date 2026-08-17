package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runDirectCodingApplicationInterpreter(
	runtime typedWorkerRuntime,
	intentModel string,
	surfaceModel string,
	artifactModel string,
	authority string,
	applicationContext assemblyline.ApplicationContext,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationSpecification, error) {
	var zero assemblyline.ApplicationSpecification

	formalizedContext, err := resolveDirectCodingApplicationContext(
		runtime, intentModel, authority, applicationContext, identities, nil,
	)
	if err != nil {
		return zero, err
	}
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
	resolution, err := resolveDirectCodingApplicationIntent(
		runtime, intentModel,
		assemblyline.ApplicationIntentInput{
			UserRequest: authority, Context: formalizedContext,
		},
		identities,
	)
	if err != nil {
		return zero, err
	}
	artifacts, err := classifyArtifactHandling(runtime, artifactModel, authority, identities)
	if err != nil {
		return zero, err
	}
	requirements := make([]assemblyline.Requirement, len(resolution.Requirements))
	for index, requirement := range resolution.Requirements {
		requirements[index] = assemblyline.Requirement{
			ID: requirement.ID, SourceQuote: requirement.Statement,
		}
	}
	specification := assemblyline.ApplicationSpecification{
		Surface: classification.Surface, ProductQuote: resolution.ProductContext,
		Requirements: requirements, Artifacts: artifacts,
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
