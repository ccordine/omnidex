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
	anchors := make([]string, 0, assemblyline.MaxRepositorySearchAnchorLeaves)
	for {
		leafInput := assemblyline.RepositorySearchAnchorLeafInput{
			UnresolvedConcept: unresolvedConcept,
			AcceptedAnchors:   append([]string{}, anchors...),
		}
		anchorJob, err := assemblyline.NewRepositorySearchAnchorJob(leafInput)
		if err != nil {
			return assemblyline.RepositorySearchTermDecision{}, err
		}
		anchor, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, "repository_search_anchor", anchorJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeRepositorySearchAnchorLeaf(leafInput, raw)
			},
			func(value string) error {
				return assemblyline.ValidatePathFreeModelContextWithProvenance(
					"repository search anchor", runtime.PathProvenance, value,
				)
			},
		)
		if err != nil {
			return assemblyline.RepositorySearchTermDecision{}, err
		}
		anchors = append(anchors, anchor)
		leafInput.AcceptedAnchors = append([]string{}, anchors...)
		coverageJob, err := assemblyline.NewRepositorySearchAnchorCoverageJob(leafInput)
		if err != nil {
			return assemblyline.RepositorySearchTermDecision{}, err
		}
		coverage, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, "repository_search_anchor_coverage",
			coverageJob, identities,
			func(raw string) (string, error) {
				return assemblyline.DecodeRepositorySearchAnchorCoverageLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		if err != nil {
			return assemblyline.RepositorySearchTermDecision{}, err
		}
		if coverage == assemblyline.RepositoryNoUncoveredAnchor {
			return assemblyline.AssembleRepositorySearchTermDecision(input, anchors)
		}
		if len(anchors) == assemblyline.MaxRepositorySearchAnchorLeaves {
			return assemblyline.RepositorySearchTermDecision{}, fmt.Errorf(
				"repository search anchor coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxRepositorySearchAnchorLeaves,
			)
		}
	}
}

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
