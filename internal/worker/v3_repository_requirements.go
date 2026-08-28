package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func interpretRepositoryRequirements(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	context assemblyline.ApplicationContext,
	identities []assemblyline.ArtifactIdentity,
) ([]string, error) {
	input := assemblyline.RepositoryRequirementInterpretationInput{
		UserRequest: authority, Context: context,
	}
	requirements := make([]string, 0, assemblyline.MaxRepositoryRequirementLeaves)
	for {
		leafInput := assemblyline.RepositoryRequirementLeafInput{
			Authority:            input,
			AcceptedRequirements: append([]string{}, requirements...),
		}
		if len(requirements) > 0 {
			coverageJob, err := assemblyline.NewRepositoryRequirementCoverageJob(leafInput)
			if err != nil {
				return nil, err
			}
			coverage, err := runDirectCodingSemanticLeafCall(
				runtime, modelName, "repository_requirement_coverage", coverageJob, identities,
				func(raw string) (string, error) {
					return assemblyline.DecodeRepositoryRequirementCoverageLeaf(leafInput, raw)
				},
				func(string) error { return nil },
			)
			if err != nil {
				return nil, err
			}
			if coverage == assemblyline.RepositoryNoUncoveredRequirement {
				break
			}
		}
		if len(requirements) == assemblyline.MaxRepositoryRequirementLeaves {
			return nil, fmt.Errorf(
				"repository requirement coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxRepositoryRequirementLeaves,
			)
		}
		requirementJob, err := assemblyline.NewRepositoryRequirementJob(leafInput)
		if err != nil {
			return nil, err
		}
		requirement, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, "repository_requirement", requirementJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeRepositoryRequirementLeaf(leafInput, raw)
			},
			func(value string) error {
				return assemblyline.ValidatePathFreeModelContextWithProvenance(
					"repository requirement", runtime.PathProvenance, value,
				)
			},
		)
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, requirement)
	}
	interpretation := assemblyline.RepositoryRequirementInterpretation{
		Schema:       assemblyline.RepositoryRequirementInterpretationSchemaV3,
		Requirements: requirements,
	}
	return assemblyline.ResolveRepositoryRequirements(input, interpretation)
}
