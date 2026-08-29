package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func selectExistingRepositoryChangeSurface(
	runtime typedWorkerRuntime,
	modelName, researchNeed string,
	requirementQuotes []string,
	evidence repositoryretrieval.EvidencePack,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.RepositoryChangeSurfaceDecision, error) {
	input := assemblyline.RepositoryChangeSurfaceInput{
		ResearchNeed: researchNeed,
		Requirements: append([]string(nil), requirementQuotes...),
		Evidence:     evidence,
	}
	decision := assemblyline.RepositoryChangeSurfaceDecision{
		Schema:  assemblyline.RepositoryChangeSurfaceSchemaV2,
		Targets: []assemblyline.RepositoryChangeTarget{},
	}
	for index, requirement := range input.Requirements {
		ownerInput := assemblyline.RepositoryChangeOwnerInput{
			Authority: input, FocusedRequirement: requirement,
		}
		job, err := assemblyline.NewRepositoryChangeOwnerJob(ownerInput)
		if err != nil {
			return assemblyline.RepositoryChangeSurfaceDecision{}, err
		}
		owner, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, fmt.Sprintf("repository_change_owner_%03d", index+1),
			job, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeRepositoryChangeOwnerLeaf(ownerInput, raw)
			},
			func(string) error { return nil },
		)
		if err != nil {
			return assemblyline.RepositoryChangeSurfaceDecision{}, err
		}
		if owner == assemblyline.RepositoryChangeOwnerNone {
			continue
		}
		decision.Targets = append(decision.Targets, assemblyline.RepositoryChangeTarget{
			SymbolID: owner, Requirement: requirement,
		})
	}
	if err := decision.ValidateFor(input); err != nil {
		return assemblyline.RepositoryChangeSurfaceDecision{}, err
	}
	return decision, nil
}

func selectExistingRepositoryRequirementSurface(
	runtime typedWorkerRuntime,
	modelName string,
	acquisition existingRepositoryEvidenceAcquisition,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.RepositoryChangeSurfaceDecision, error) {
	if acquisition.RequirementQuote == "" {
		return assemblyline.RepositoryChangeSurfaceDecision{}, fmt.Errorf(
			"repository requirement surface requires one exact requirement quote",
		)
	}
	return selectExistingRepositoryChangeSurface(
		runtime, modelName, acquisition.RequirementQuote,
		[]string{acquisition.RequirementQuote}, acquisition.Pack, identities,
	)
}
