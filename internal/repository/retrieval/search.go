package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

type scoredSymbol struct {
	symbol repositoryfacts.Symbol
	score  float64
}

func (service *Service) searchCandidates(
	ctx context.Context,
	request Request,
	canonical map[string]repositoryfacts.Symbol,
) ([]scoredSymbol, error) {
	matches, err := service.store.SearchRepositorySymbols(
		ctx, request.ProjectID, request.AnalysisID, request.Query, maxSymbolSearchScan,
	)
	if err != nil {
		return nil, err
	}
	if len(matches) >= maxSymbolSearchScan {
		return nil, fmt.Errorf("repository symbol search reached its %d-result limit; narrow the query", maxSymbolSearchScan)
	}
	scores := make(map[string]float64, len(matches))
	for _, match := range matches {
		exact, exists := canonical[match.Symbol.ID]
		if !exists {
			return nil, fmt.Errorf("repository symbol search returned unknown symbol %q", match.Symbol.ID)
		}
		if match.Symbol != exact {
			return nil, fmt.Errorf("repository symbol search returned altered symbol %q", match.Symbol.ID)
		}
		prior, exists := scores[match.Symbol.ID]
		if !exists || match.Score > prior {
			scores[match.Symbol.ID] = match.Score
		}
	}
	items := make([]scoredSymbol, 0, len(scores))
	for id, score := range scores {
		items = append(items, scoredSymbol{symbol: canonical[id], score: score})
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].score == items[right].score {
			return items[left].symbol.ID < items[right].symbol.ID
		}
		return items[left].score > items[right].score
	})
	return items, nil
}

func (service *Service) resolveExactSymbol(
	ctx context.Context,
	request Request,
	canonical map[string]repositoryfacts.Symbol,
) (repositoryfacts.Symbol, error) {
	candidates, err := service.searchCandidates(ctx, request, canonical)
	if err != nil {
		return repositoryfacts.Symbol{}, err
	}
	exact := make([]repositoryfacts.Symbol, 0, 2)
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.symbol.Name, request.Query) ||
			strings.EqualFold(candidate.symbol.QualifiedName, request.Query) {
			exact = append(exact, candidate.symbol)
		}
	}
	if len(exact) == 0 {
		return repositoryfacts.Symbol{}, fmt.Errorf("%w: exact symbol %q is absent", ErrInsufficientEvidence, request.Query)
	}
	if len(exact) > 1 {
		return repositoryfacts.Symbol{}, fmt.Errorf("repository symbol query %q is ambiguous across %d exact declarations", request.Query, len(exact))
	}
	return exact[0], nil
}

func candidateIDs(candidates []scoredSymbol) []string {
	ids := make([]string, len(candidates))
	for index, candidate := range candidates {
		ids[index] = candidate.symbol.ID
	}
	sort.Strings(ids)
	return ids
}
