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
	for {
		leafInput := assemblyline.ApplicationRequirementLeafInput{
			UserRequest: authority.UserRequest, Context: authority.Context,
			ProductContext:       productContext,
			AcceptedRequirements: append([]string{}, requirements...),
		}
		if len(requirements) > 0 {
			coverageJob, err := assemblyline.NewApplicationRequirementCoverageJob(leafInput)
			if err != nil {
				return zero, err
			}
			coverage, err := runDirectCodingSemanticLeafCall(
				runtime, intentModel, "application_requirement_coverage", coverageJob, identities,
				func(raw string) (string, error) {
					return assemblyline.DecodeApplicationRequirementCoverageLeaf(leafInput, raw)
				},
				func(string) error { return nil },
			)
			if err != nil {
				return zero, err
			}
			if coverage == assemblyline.ApplicationNoUncoveredRequirement {
				candidate := assemblyline.ApplicationIntentCandidate{
					Schema:         assemblyline.ApplicationIntentCandidateSchemaV1,
					ProductContext: productContext,
					Requirements:   append([]string(nil), requirements...),
				}
				return assemblyline.ResolveApplicationIntent(authority, candidate)
			}
		}
		if len(requirements) == assemblyline.MaxApplicationRequirementLeaves {
			return zero, fmt.Errorf(
				"application requirement coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxApplicationRequirementLeaves,
			)
		}
		requirementJob, err := assemblyline.NewApplicationRequirementJob(leafInput)
		if err != nil {
			return zero, err
		}
		requirement, err := runDirectCodingSemanticLeafCall(
			runtime, intentModel, "application_requirement", requirementJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeApplicationRequirementLeaf(leafInput, raw)
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
		requirements = append(requirements, requirement)
	}
}
