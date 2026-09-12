package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingApplicationRequestAuthority keeps the actual request provenance
// code-only while exposing only the artifact-redacted request to semantic jobs.
type directCodingApplicationRequestAuthority struct {
	authoritativeRequest string
	modelRequest         string
}

func newDirectCodingApplicationRequestAuthority(
	authoritativeRequest string,
	modelRequest string,
) (directCodingApplicationRequestAuthority, error) {
	var zero directCodingApplicationRequestAuthority
	authority := directCodingApplicationRequestAuthority{
		authoritativeRequest: authoritativeRequest,
		modelRequest:         modelRequest,
	}
	if err := authority.validate(); err != nil {
		return zero, err
	}
	return authority, nil
}

func (authority directCodingApplicationRequestAuthority) validate() error {
	if strings.TrimSpace(authority.authoritativeRequest) == "" {
		return fmt.Errorf("direct coding application authority has an invalid authoritative request")
	}
	if strings.TrimSpace(authority.modelRequest) == "" {
		return fmt.Errorf("direct coding application authority has an invalid model request")
	}
	if err := assemblyline.ValidatePathFreeModelContext(
		"direct coding application model request", authority.modelRequest,
	); err != nil {
		return err
	}
	return nil
}

type directCodingApplicationInterpretation struct {
	Specification        assemblyline.ApplicationSpecification
	AcceptedRequirements []assemblyline.ApplicationRequirement
}

func (interpretation directCodingApplicationInterpretation) validate() error {
	if len(interpretation.Specification.Requirements) == 0 {
		if len(interpretation.AcceptedRequirements) != 0 {
			return fmt.Errorf("filesystem-only interpretation carries unused requirement authority")
		}
		if interpretation.Specification.Surface != "" || interpretation.Specification.ProductQuote != "" {
			return fmt.Errorf("filesystem-only interpretation carries unused application authority")
		}
		return nil
	}
	if len(interpretation.AcceptedRequirements) != len(interpretation.Specification.Requirements) {
		return fmt.Errorf("application interpretation result-relation receipts do not cover accepted requirements")
	}
	for index, accepted := range interpretation.AcceptedRequirements {
		requirement := interpretation.Specification.Requirements[index]
		if accepted.ID != requirement.ID || accepted.Statement != requirement.SourceQuote {
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
	intentModels directCodingApplicationIntentModels,
	surfaceModel func() (string, error),
	artifactModel func() (string, error),
	authority directCodingApplicationRequestAuthority,
	applicationContext assemblyline.ApplicationContext,
	approvedRequirements []assemblyline.ApplicationRequirement,
	identities []assemblyline.ArtifactIdentity,
) (directCodingApplicationInterpretation, error) {
	var zero directCodingApplicationInterpretation

	resolution, err := resolveApprovedDirectCodingApplicationIntent(
		runtime, intentModels,
		assemblyline.ApplicationIntentInput{
			UserRequest: authority.modelRequest, Context: applicationContext,
		},
		approvedRequirements,
		identities,
	)
	if err != nil {
		return zero, err
	}
	artifacts := []assemblyline.ArtifactDirective(nil)
	if len(identities) > 0 {
		modelName, err := artifactModel()
		if err != nil {
			return zero, err
		}
		artifacts, err = classifyArtifactHandling(
			runtime, modelName, authority.modelRequest, identities,
		)
		if err != nil {
			return zero, err
		}
	}
	artifacts, err = sieveDirectCodingApplicationArtifactDirectives(artifacts)
	if err != nil {
		return zero, err
	}
	if len(resolution.Requirements) == 0 {
		interpretation := directCodingApplicationInterpretation{
			Specification: assemblyline.ApplicationSpecification{Artifacts: artifacts},
		}
		if err := interpretation.validate(); err != nil {
			return zero, err
		}
		return interpretation, nil
	}
	modelName, err := surfaceModel()
	if err != nil {
		return zero, err
	}
	classification, err := classifyApplicationSurface(
		runtime, modelName, authority.modelRequest, identities,
	)
	if err != nil {
		return zero, err
	}
	surface, err := assemblyline.ResolveApplicationSurface(classification)
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
		Surface: surface, ProductQuote: resolution.ProductContext,
		Requirements: requirements, Artifacts: artifacts,
	}
	interpretation := directCodingApplicationInterpretation{
		Specification: specification,
	}
	accepted := append([]assemblyline.ApplicationRequirement(nil), resolution.Requirements...)
	interpretation.AcceptedRequirements = accepted
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
	)
}
