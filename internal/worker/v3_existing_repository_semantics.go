package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func classifyExistingRepositoryRetrieval(
	runtime typedWorkerRuntime,
	modelName, researchNeed string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.RepositoryRetrievalDecision, error) {
	input := assemblyline.RepositoryRetrievalInput{ResearchNeed: researchNeed}
	job, err := assemblyline.NewRepositoryRetrievalJob(input)
	if err != nil {
		return assemblyline.RepositoryRetrievalDecision{}, err
	}
	return runDirectCodingSemanticCall[assemblyline.RepositoryRetrievalDecision](
		runtime, modelName, "repository_retrieval", job, identities,
		func(value assemblyline.RepositoryRetrievalDecision) error { return value.ValidateFor(input) },
	)
}

func selectExistingRepositoryChangeSurface(
	runtime typedWorkerRuntime,
	modelName, researchNeed string,
	requirementQuotes []string,
	evidence repositoryretrieval.EvidencePack,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.RepositoryChangeSurfaceDecision, error) {
	input := assemblyline.RepositoryChangeSurfaceInput{
		ResearchNeed: researchNeed, RequirementQuotes: append([]string(nil), requirementQuotes...),
		Evidence: evidence,
	}
	job, err := assemblyline.NewRepositoryChangeSurfaceJob(input)
	if err != nil {
		return assemblyline.RepositoryChangeSurfaceDecision{}, err
	}
	decision, err := runDirectCodingSemanticCall[assemblyline.RepositoryChangeSurfaceDecision](
		runtime, modelName, "repository_change_surface", job, identities,
		func(value assemblyline.RepositoryChangeSurfaceDecision) error { return value.ValidateFor(input) },
	)
	if err != nil {
		return assemblyline.RepositoryChangeSurfaceDecision{}, err
	}
	if len(decision.UnresolvedRequirementQuotes) > 0 {
		return assemblyline.RepositoryChangeSurfaceDecision{}, fmt.Errorf(
			"insufficient_repository_evidence: unresolved requirements: %v",
			decision.UnresolvedRequirementQuotes,
		)
	}
	return decision, nil
}
