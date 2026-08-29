package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationInterpretation struct {
	Specification        assemblyline.ApplicationSpecification
	AcceptedRequirements []assemblyline.ApplicationRequirement
}

func (interpretation directCodingApplicationInterpretation) validate() error {
	if err := interpretation.Specification.Validate(); err != nil {
		return err
	}
	if len(interpretation.AcceptedRequirements) != len(interpretation.Specification.Requirements) {
		return fmt.Errorf("application interpretation result-relation receipts do not cover accepted requirements")
	}
	for index, accepted := range interpretation.AcceptedRequirements {
		requirement := interpretation.Specification.Requirements[index]
		if accepted.ID != requirement.ID || accepted.Statement != requirement.SourceQuote ||
			accepted.RequestSHA256 == "" {
			return fmt.Errorf(
				"application interpretation requirement %d differs from accepted specification authority",
				index,
			)
		}
		if err := accepted.ResultRelation.ValidateAcceptedFor(accepted.Statement); err != nil {
			return fmt.Errorf(
				"application interpretation requirement %s result relation: %w",
				accepted.ID, err,
			)
		}
	}
	return nil
}

func runDirectCodingApplicationInterpreter(
	runtime typedWorkerRuntime,
	intentModel string,
	surfaceModel string,
	artifactModel string,
	authority string,
	applicationContext assemblyline.ApplicationContext,
	identities []assemblyline.ArtifactIdentity,
) (directCodingApplicationInterpretation, error) {
	var zero directCodingApplicationInterpretation

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
	interpretation := directCodingApplicationInterpretation{
		Specification: specification,
		AcceptedRequirements: append(
			[]assemblyline.ApplicationRequirement(nil), resolution.Requirements...,
		),
	}
	if err := interpretation.validate(); err != nil {
		return zero, err
	}
	return interpretation, nil
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
	return runDirectCodingSemanticLeafCall(
		runtime, modelName, "application_surface", job, identities,
		func(raw string) (assemblyline.ApplicationClassification, error) {
			return assemblyline.DecodeApplicationClassification(jobInput, raw)
		},
		func(value assemblyline.ApplicationClassification) error { return value.Validate() },
	)
}
