package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

const maxExistingRepositoryDeterministicQueries = 8

type existingRepositoryEvidenceBuild func(string) (repositoryretrieval.EvidencePack, error)
type existingRepositorySearchTermCall func(string) (assemblyline.RepositorySearchTermDecision, error)
type existingRepositoryEvidenceConsumer func(existingRepositoryEvidenceAcquisition) error
type existingRepositoryChangeSurfaceCall func(
	existingRepositoryEvidenceAcquisition,
) (assemblyline.RepositoryChangeSurfaceDecision, error)

type existingRepositoryEvidenceAcquisition struct {
	Need             assemblyline.ApplicationEvidenceNeed
	RequirementQuote string
	Pack             repositoryretrieval.EvidencePack
	Query            string
	SearchTermCalls  int
}

type existingRepositoryRequirementResolution struct {
	Acquisition existingRepositoryEvidenceAcquisition
	Surface     assemblyline.RepositoryChangeSurfaceDecision
}

func acquireExistingRepositoryEvidence(
	requirementQuotes []string,
	build existingRepositoryEvidenceBuild,
	searchTerm existingRepositorySearchTermCall,
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
	seen := make(map[string]struct{}, len(requirementQuotes))
	needs := make([]assemblyline.ApplicationEvidenceNeed, len(requirementQuotes))
	for index, query := range requirementQuotes {
		need, err := assemblyline.NewApplicationRepositoryChangeOwnerNeed(index+1, query)
		if err != nil {
			return nil, err
		}
		needs[index] = need
		if _, err := repositoryretrieval.NewQueryBinding(
			repositoryretrieval.OperationSemanticExcerpts, query,
		); err != nil {
			return nil, fmt.Errorf("repository deterministic query: %w", err)
		}
		if _, duplicate := seen[query]; duplicate {
			return nil, fmt.Errorf("repository deterministic queries must be unique")
		}
		seen[query] = struct{}{}
	}
	results := make([]existingRepositoryEvidenceAcquisition, len(requirementQuotes))
	missing := make([]int, 0, len(requirementQuotes))
	for index, query := range requirementQuotes {
		results[index] = existingRepositoryEvidenceAcquisition{
			Need:             needs[index],
			RequirementQuote: query,
			Query:            query,
		}
		pack, err := build(query)
		if err == nil {
			if err := pack.ValidateForRequest(
				repositoryretrieval.OperationSemanticExcerpts, query,
			); err != nil {
				return results, fmt.Errorf(
					"repository deterministic acquisition for %q returned invalid evidence: %w",
					query, err,
				)
			}
			results[index].Need.SearchAnchors = []string{query}
			if err := results[index].Need.Validate(); err != nil {
				return results, err
			}
			results[index].Pack = pack
			continue
		}
		if !errors.Is(err, repositoryretrieval.ErrInsufficientEvidence) {
			return results, fmt.Errorf("repository deterministic acquisition for %q: %w", query, err)
		}
		missing = append(missing, index)
	}
	if len(missing) == 0 {
		return results, nil
	}
	if searchTerm == nil {
		return results, fmt.Errorf(
			"%w: deterministic repository queries exhausted and search-term station is unavailable",
			repositoryretrieval.ErrInsufficientEvidence,
		)
	}
	for _, index := range missing {
		result := &results[index]
		result.SearchTermCalls = 1
		decision, err := searchTerm(result.RequirementQuote)
		if err != nil {
			return results, err
		}
		searchInput := assemblyline.RepositorySearchTermInput{
			UnresolvedConcept: result.RequirementQuote,
		}
		if err := decision.ValidateFor(searchInput); err != nil {
			return results, err
		}
		result.Need.SearchAnchors = append([]string(nil), decision.Anchors...)
		if err := result.Need.Validate(); err != nil {
			return results, err
		}
		query, err := repositoryretrieval.BuildLexicalAnchorQuery(decision.Anchors)
		if err != nil {
			return results, err
		}
		pack, buildErr := build(query)
		if buildErr != nil {
			return results, fmt.Errorf(
				"repository acquisition for requirement %q bounded lexical anchor query: %w",
				result.RequirementQuote, buildErr,
			)
		}
		if err := pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts, query,
		); err != nil {
			return results, fmt.Errorf(
				"repository acquisition for requirement %q bounded lexical anchor query returned invalid evidence: %w",
				result.RequirementQuote, err,
			)
		}
		result.Pack, result.Query = pack, query
	}
	return results, nil
}

func prepareExistingRepositoryRequirementResolutions(
	requirementQuotes []string,
	build existingRepositoryEvidenceBuild,
	searchTerm existingRepositorySearchTermCall,
	consume existingRepositoryEvidenceConsumer,
	resolveSurface existingRepositoryChangeSurfaceCall,
) ([]existingRepositoryRequirementResolution, error) {
	if consume == nil {
		return nil, fmt.Errorf("repository requirement closure requires one evidence consumer")
	}
	if resolveSurface == nil {
		return nil, fmt.Errorf("repository requirement closure requires one bounded change-surface station")
	}
	acquisitions, err := acquireExistingRepositoryEvidence(requirementQuotes, build, searchTerm)
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
