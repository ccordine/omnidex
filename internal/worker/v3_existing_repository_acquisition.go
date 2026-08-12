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
	for _, query := range requirementQuotes {
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
		if decision.Schema != assemblyline.RepositorySearchTermSchemaV1 {
			return results, fmt.Errorf(
				"repository search term schema must be %q", assemblyline.RepositorySearchTermSchemaV1,
			)
		}
		if _, err := repositoryretrieval.NewQueryBinding(
			repositoryretrieval.OperationSemanticExcerpts, decision.Term,
		); err != nil {
			return results, fmt.Errorf("repository model search term: %w", err)
		}
		pack, err := build(decision.Term)
		if err != nil {
			return results, fmt.Errorf(
				"repository acquisition for requirement %q bounded model search term: %w",
				result.RequirementQuote, err,
			)
		}
		if err := pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts, decision.Term,
		); err != nil {
			return results, fmt.Errorf(
				"repository acquisition for requirement %q bounded model search term returned invalid evidence: %w",
				result.RequirementQuote, err,
			)
		}
		result.Pack, result.Query = pack, decision.Term
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
	for _, acquisition := range acquisitions[1:] {
		if acquisition.Pack.SnapshotID != snapshotID || acquisition.Pack.AnalysisID != analysisID {
			return fmt.Errorf("repository requirement evidence must share one snapshot and analysis")
		}
	}
	return nil
}
