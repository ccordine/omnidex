package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runDirectCodingApplicationInterpreter(
	runtime typedWorkerRuntime,
	partitionModel string,
	surfaceModel string,
	identityModel string,
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
	identity, err := extractApplicationIdentity(runtime, identityModel, authority, identities)
	if err != nil {
		return zero, err
	}
	requirements, err := extractGroundedRequirements(runtime, partitionModel, authority, identities)
	if err != nil {
		return zero, err
	}
	artifacts, err := classifyArtifactHandling(runtime, artifactModel, authority, identities)
	if err != nil {
		return zero, err
	}
	specification := assemblyline.ApplicationSpecification{
		Surface: classification.Surface, ProductQuote: identity.ProductQuote,
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

func extractApplicationIdentity(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationIdentity, error) {
	input := assemblyline.ApplicationIdentityInput{UserRequest: authority}
	job, err := assemblyline.NewApplicationIdentityJob(input)
	if err != nil {
		return assemblyline.ApplicationIdentity{}, err
	}
	return runDirectCodingSemanticCall[assemblyline.ApplicationIdentity](
		runtime, modelName, "application_identity", job, identities,
		func(value assemblyline.ApplicationIdentity) error { return value.ValidateFor(input) },
	)
}

func extractGroundedRequirements(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) ([]assemblyline.Requirement, error) {
	partition, err := partitionCodingRequirements(runtime, modelName, authority, identities)
	if err != nil {
		return nil, err
	}
	graph, err := assemblyline.BuildRequirementGraph(authority, partition.FeatureQuotes)
	if err != nil {
		return nil, err
	}
	return graph.Requirements, nil
}
