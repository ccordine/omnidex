package retrieval

import (
	"context"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (service *Service) buildSemanticExcerpts(
	ctx context.Context,
	request Request,
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
	canonical map[string]repositoryfacts.Symbol,
	pack *EvidencePack,
) error {
	candidates, err := service.searchCandidates(ctx, request, canonical)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("%w: semantic query matched no indexed symbols", ErrInsufficientEvidence)
	}
	seedCount := len(candidates)
	if seedCount > request.Limits.MaxSymbols {
		seedCount = request.Limits.MaxSymbols
	}
	seeds := candidates[:seedCount]
	edges, err := service.boundedNeighborhood(ctx, request, analysis, candidateIDs(seeds))
	if err != nil {
		return err
	}
	selected := selectEvidenceSymbols(seeds, edges, canonical, request.Limits.MaxSymbols)
	selectedIDs := scoredSymbolIDs(selected)
	for _, candidate := range candidates[seedCount:] {
		if err := addOmittedSymbol(pack, candidate.symbol.ID); err != nil {
			return err
		}
	}
	for _, edge := range edges {
		for _, id := range []string{edge.FromID, edge.ToID} {
			if _, isSymbol := canonical[id]; !isSymbol {
				continue
			}
			if _, included := selectedIDs[id]; !included {
				if err := addOmittedSymbol(pack, id); err != nil {
					return err
				}
			}
		}
	}
	if err := addEvidenceSymbols(pack, snapshot, selected, request.Limits); err != nil {
		return err
	}
	addEvidenceRelations(pack, edges, selectedIDs, "")
	return nil
}

func selectEvidenceSymbols(
	candidates []scoredSymbol,
	edges []repositoryfacts.Edge,
	canonical map[string]repositoryfacts.Symbol,
	limit int,
) []scoredSymbol {
	selected := make([]scoredSymbol, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, candidate := range candidates {
		if len(selected) == limit {
			break
		}
		selected = append(selected, candidate)
		seen[candidate.symbol.ID] = struct{}{}
	}
	for _, edge := range edges {
		for _, id := range []string{edge.FromID, edge.ToID} {
			if len(selected) == limit {
				return selected
			}
			if _, exists := seen[id]; exists {
				continue
			}
			symbol, exists := canonical[id]
			if !exists {
				continue
			}
			selected = append(selected, scoredSymbol{symbol: symbol})
			seen[id] = struct{}{}
		}
	}
	return selected
}

func scoredSymbolIDs(items []scoredSymbol) map[string]struct{} {
	ids := make(map[string]struct{}, len(items))
	for _, item := range items {
		ids[item.symbol.ID] = struct{}{}
	}
	return ids
}
