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
	requirements := make(
		[]assemblyline.ApplicationIntentCandidateRequirement,
		0,
		assemblyline.MaxApplicationRequirementLeaves,
	)
	excludedCandidates := make(
		[]string, 0, assemblyline.MaxApplicationRequirementExcludedCandidates,
	)
	zeroDeltas := make(
		[]assemblyline.ApplicationRequirementCandidateZeroDelta,
		0,
		assemblyline.MaxApplicationRequirementZeroDeltas,
	)
	var reboundGenerationAuthority *assemblyline.ApplicationRequirementCandidateInput
	for {
		var candidateInput assemblyline.ApplicationRequirementCandidateInput
		if reboundGenerationAuthority != nil {
			candidateInput = *reboundGenerationAuthority
			reboundGenerationAuthority = nil
		} else {
			acceptedStatements := make([]string, len(requirements))
			for index, requirement := range requirements {
				acceptedStatements[index] = requirement.Statement
			}
			coverageInput := assemblyline.ApplicationRequirementCoverageInput{
				UserRequest: authority.UserRequest, Context: authority.Context,
				AcceptedRequirements: acceptedStatements,
				ExcludedCandidates:   append([]string{}, excludedCandidates...),
				ZeroDeltas: append(
					[]assemblyline.ApplicationRequirementCandidateZeroDelta{},
					zeroDeltas...,
				),
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
					Requirements: append(
						[]assemblyline.ApplicationIntentCandidateRequirement(nil),
						requirements...,
					),
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
			runtime, intentModel, requirement, candidateInput, requirements, identities,
		)
		if err != nil {
			return zero, err
		}
		if !resolved.Retain {
			if resolved.ResultRelation != (assemblyline.ApplicationRequirementCandidateResultRelationResult{}) {
				return zero, fmt.Errorf(
					"unretained application requirement unexpectedly carries a result-relation receipt",
				)
			}
			if resolved.ZeroDelta != nil {
				if resolved.ReboundGenerationAuthority != nil {
					return zero, fmt.Errorf(
						"zero-delta application requirement unexpectedly carries exclusion authority",
					)
				}
				rebound, err := assemblyline.RecordApplicationRequirementCandidateZeroDelta(
					candidateInput, *resolved.ZeroDelta,
				)
				if err != nil {
					return zero, err
				}
				zeroDeltas = append(
					[]assemblyline.ApplicationRequirementCandidateZeroDelta{},
					rebound.ZeroDeltas...,
				)
				continue
			}
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
		if resolved.ZeroDelta != nil {
			return zero, fmt.Errorf(
				"retained application requirement unexpectedly carries zero-delta evidence",
			)
		}
		if err := resolved.ResultRelation.ValidateAcceptedFor(resolved.Candidate); err != nil {
			return zero, fmt.Errorf("retained application requirement result relation: %w", err)
		}
		requirements = append(requirements, assemblyline.ApplicationIntentCandidateRequirement{
			Statement: resolved.Candidate, ResultRelation: resolved.ResultRelation,
		})
	}
}
