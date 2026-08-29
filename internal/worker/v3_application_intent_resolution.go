package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func resolveDirectCodingApplicationIntent(
	runtime typedWorkerRuntime,
	intentModel string,
	authority assemblyline.ApplicationIntentInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationIntentResolution, error) {
	var zero assemblyline.ApplicationIntentResolution
	productInput := assemblyline.ApplicationProductContextInput{
		UserRequest: authority.UserRequest,
		Context:     authority.Context,
	}
	productJob, err := assemblyline.NewApplicationProductContextJob(productInput)
	if err != nil {
		return zero, err
	}
	productContext, err := runDirectCodingSemanticLeafCall(
		runtime, intentModel, "application_product_context", productJob, identities,
		func(raw string) (string, error) {
			return assemblyline.DecodeApplicationProductContextLeaf(productInput, raw)
		},
		func(value string) error {
			return assemblyline.ValidatePathFreeModelContextWithProvenance(
				"application product context", runtime.PathProvenance, value,
			)
		},
	)
	if err != nil {
		return zero, err
	}
	requirements := make([]string, 0, assemblyline.MaxApplicationRequirementLeaves)
	excludedCandidates := make(
		[]string, 0, assemblyline.MaxApplicationRequirementExcludedCandidates,
	)
	var reboundGenerationAuthority *assemblyline.ApplicationRequirementCandidateInput
	for {
		var candidateInput assemblyline.ApplicationRequirementCandidateInput
		if reboundGenerationAuthority != nil {
			candidateInput = *reboundGenerationAuthority
			reboundGenerationAuthority = nil
		} else {
			coverageInput := assemblyline.ApplicationRequirementCoverageInput{
				UserRequest: authority.UserRequest, Context: authority.Context,
				AcceptedRequirements: append([]string{}, requirements...),
				ExcludedCandidates:   append([]string{}, excludedCandidates...),
			}
			coverageJob, err := assemblyline.NewApplicationRequirementCoverageJob(coverageInput)
			if err != nil {
				return zero, err
			}
			coverage, err := runDirectCodingSemanticLeafCall(
				runtime, intentModel, "application_requirement_coverage", coverageJob, identities,
				func(raw string) (assemblyline.ApplicationRequirementCoverageResult, error) {
					return assemblyline.DecodeApplicationRequirementCoverageLeaf(coverageInput, raw)
				},
				func(value assemblyline.ApplicationRequirementCoverageResult) error {
					return value.ValidateFor(coverageInput)
				},
			)
			if err != nil {
				return zero, err
			}
			if coverage.Relation == assemblyline.ApplicationNoUncoveredRequirement {
				if len(requirements) == 0 {
					return zero, fmt.Errorf(
						"application requirement coverage found no task-local runtime implementation requirement",
					)
				}
				candidate := assemblyline.ApplicationIntentCandidate{
					Schema:         assemblyline.ApplicationIntentCandidateSchemaV1,
					ProductContext: productContext,
					Requirements:   append([]string(nil), requirements...),
				}
				return assemblyline.ResolveApplicationIntent(authority, candidate)
			}
			candidateInput = assemblyline.ApplicationRequirementCandidateInput{
				Authority: coverageInput,
				Coverage:  coverage,
			}
		}
		if len(requirements) == assemblyline.MaxApplicationRequirementLeaves {
			return zero, fmt.Errorf(
				"application requirement coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxApplicationRequirementLeaves,
			)
		}
		requirementJob, err := assemblyline.NewApplicationRequirementJob(candidateInput)
		if err != nil {
			return zero, err
		}
		requirement, err := runDirectCodingSemanticLeafCall(
			runtime, intentModel, "application_requirement", requirementJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationRequirementLeaf(candidateInput, raw)
			},
			func(value string) error {
				return assemblyline.ValidatePathFreeModelContextWithProvenance(
					"application requirement", runtime.PathProvenance, value,
				)
			},
		)
		if err != nil {
			return zero, err
		}
		resolved, err := resolveDirectCodingApplicationRequirementCandidate(
			runtime, intentModel, requirement, candidateInput, identities,
		)
		if err != nil {
			return zero, err
		}
		if !resolved.Retain {
			if len(excludedCandidates) == assemblyline.MaxApplicationRequirementExcludedCandidates {
				return zero, fmt.Errorf(
					"application requirement exclusions reached the code-owned %d-item bound",
					assemblyline.MaxApplicationRequirementExcludedCandidates,
				)
			}
			if resolved.ReboundGenerationAuthority == nil {
				return zero, fmt.Errorf(
					"non-runtime application requirement lacks rebound generation authority",
				)
			}
			excludedCandidates = append(excludedCandidates, resolved.Candidate)
			reboundGenerationAuthority = resolved.ReboundGenerationAuthority
			continue
		}
		if resolved.ReboundGenerationAuthority != nil {
			return zero, fmt.Errorf(
				"retained application requirement unexpectedly carries exclusion authority",
			)
		}
		requirements = append(requirements, resolved.Candidate)
	}
}
