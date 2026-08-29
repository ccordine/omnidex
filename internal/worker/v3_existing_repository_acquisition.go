package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

const maxExistingRepositoryDeterministicQueries = 8

type existingRepositoryEvidenceBuild func(string) (repositoryretrieval.EvidencePack, error)
type existingRepositoryEvidenceConsumer func(existingRepositoryEvidenceAcquisition) error
type existingRepositoryChangeSurfaceCall func(
	existingRepositoryEvidenceAcquisition,
) (assemblyline.RepositoryChangeSurfaceDecision, error)

type existingRepositoryEvidenceAcquisition struct {
	Need             assemblyline.ApplicationEvidenceNeed
	RequirementQuote string
	Pack             repositoryretrieval.EvidencePack
	Query            string
}

type existingRepositoryRequirementResolution struct {
	Acquisition existingRepositoryEvidenceAcquisition
	Surface     assemblyline.RepositoryChangeSurfaceDecision
}

func acquireExistingRepositoryEvidence(
	requirementQuotes []string,
	codeOwnedQuery string,
	build existingRepositoryEvidenceBuild,
) ([]existingRepositoryEvidenceAcquisition, error) {
	if len(requirementQuotes) < 1 || len(requirementQuotes) > maxExistingRepositoryDeterministicQueries {
		return nil, fmt.Errorf(
			"repository evidence closure requires 1-%d code-held requirement queries",
			maxExistingRepositoryDeterministicQueries,
		)
	}
	if build == nil {
		return nil, fmt.Errorf("repository evidence closure requires one code-owned acquisition operation")
	}
	if _, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts, codeOwnedQuery,
	); err != nil {
		return nil, fmt.Errorf("repository evidence code-owned query: %w", err)
	}
	seen := make(map[string]struct{}, len(requirementQuotes))
	needs := make([]assemblyline.ApplicationEvidenceNeed, len(requirementQuotes))
	for index, query := range requirementQuotes {
		need, err := assemblyline.NewApplicationRepositoryChangeOwnerNeed(index+1, query)
		if err != nil {
			return nil, err
		}
		needs[index] = need
		if _, duplicate := seen[query]; duplicate {
			return nil, fmt.Errorf("repository semantic requirements must be unique")
		}
		seen[query] = struct{}{}
	}
	pack, err := build(codeOwnedQuery)
	if err != nil {
		return nil, fmt.Errorf("repository deterministic acquisition: %w", err)
	}
	if err := pack.ValidateForRequest(
		repositoryretrieval.OperationSemanticExcerpts, codeOwnedQuery,
	); err != nil {
		return nil, fmt.Errorf("repository deterministic acquisition returned invalid evidence: %w", err)
	}
	results := make([]existingRepositoryEvidenceAcquisition, len(requirementQuotes))
	for index, requirement := range requirementQuotes {
		results[index] = existingRepositoryEvidenceAcquisition{
			Need:             needs[index],
			RequirementQuote: requirement,
			Pack:             pack,
			Query:            codeOwnedQuery,
		}
	}
	return results, nil
}

func prepareExistingRepositoryRequirementResolutions(
	requirementQuotes []string,
	codeOwnedQuery string,
	build existingRepositoryEvidenceBuild,
	consume existingRepositoryEvidenceConsumer,
	resolveSurface existingRepositoryChangeSurfaceCall,
) ([]existingRepositoryRequirementResolution, error) {
	if consume == nil {
		return nil, fmt.Errorf("repository requirement closure requires one evidence consumer")
	}
	if resolveSurface == nil {
		return nil, fmt.Errorf("repository requirement closure requires one bounded change-surface station")
	}
	acquisitions, err := acquireExistingRepositoryEvidence(
		requirementQuotes, codeOwnedQuery, build,
	)
	if err != nil {
		return nil, err
	}
	if err := validateExistingRepositoryAcquisitionAuthority(acquisitions); err != nil {
		return nil, err
	}
	for _, acquisition := range acquisitions {
		if err := consume(acquisition); err != nil {
			return nil, err
		}
	}
	resolutions := make([]existingRepositoryRequirementResolution, 0, len(acquisitions))
	for _, acquisition := range acquisitions {
		surface, err := resolveSurface(acquisition)
		if err != nil {
			return nil, err
		}
		surfaceInput := assemblyline.RepositoryChangeSurfaceInput{
			ResearchNeed: acquisition.RequirementQuote,
			Requirements: []string{acquisition.RequirementQuote},
			Evidence:     acquisition.Pack,
		}
		unresolved, err := surface.UnresolvedRequirements(surfaceInput)
		if err != nil {
			return nil, err
		}
		if len(unresolved) != 0 || len(surface.Targets) == 0 {
			return nil, fmt.Errorf(
				"application evidence need %q did not satisfy stop condition %q",
				acquisition.Need.ID, acquisition.Need.StopCondition,
			)
		}
		resolutions = append(resolutions, existingRepositoryRequirementResolution{
			Acquisition: acquisition,
			Surface:     surface,
		})
	}
	return resolutions, nil
}

func validateExistingRepositoryAcquisitionAuthority(
	acquisitions []existingRepositoryEvidenceAcquisition,
) error {
	if len(acquisitions) == 0 {
		return fmt.Errorf("repository requirement closure acquired no evidence")
	}
	snapshotID := acquisitions[0].Pack.SnapshotID
	analysisID := acquisitions[0].Pack.AnalysisID
	for _, acquisition := range acquisitions {
		if err := acquisition.Need.Validate(); err != nil {
			return err
		}
		if acquisition.Need.Kind != assemblyline.ApplicationEvidenceChangeOwner ||
			acquisition.Need.Question != acquisition.RequirementQuote ||
			acquisition.Need.StopCondition != assemblyline.ApplicationEvidenceOwnerResolved {
			return fmt.Errorf(
				"repository requirement evidence need %q does not bind its exact requirement",
				acquisition.Need.ID,
			)
		}
		if acquisition.Pack.SnapshotID != snapshotID || acquisition.Pack.AnalysisID != analysisID {
			return fmt.Errorf("repository requirement evidence must share one snapshot and analysis")
		}
	}
	return nil
}
