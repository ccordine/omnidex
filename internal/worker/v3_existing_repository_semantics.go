package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func generateExistingRepositorySearchTerm(
	runtime typedWorkerRuntime,
	modelName, unresolvedConcept string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.RepositorySearchTermDecision, error) {
	input := assemblyline.RepositorySearchTermInput{UnresolvedConcept: unresolvedConcept}
	job, err := assemblyline.NewRepositorySearchTermJob(input)
	if err != nil {
		return assemblyline.RepositorySearchTermDecision{}, err
	}
	return runDirectCodingSemanticCall[assemblyline.RepositorySearchTermDecision](
		runtime, modelName, "repository_search_term", job, identities,
		func(value assemblyline.RepositorySearchTermDecision) error { return value.ValidateFor(input) },
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
