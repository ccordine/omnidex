package retrieval

import (
	"context"
	"sort"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (service *Service) buildSymbolDeclaration(
	ctx context.Context,
	request Request,
	snapshot repositoryfacts.Snapshot,
	canonical map[string]repositoryfacts.Symbol,
	pack *EvidencePack,
) error {
	target, err := service.resolveExactSymbol(ctx, request, canonical)
	if err != nil {
		return err
	}
	pack.SubjectSymbolID = target.ID
	return addEvidenceSymbols(pack, snapshot, []scoredSymbol{{symbol: target, score: 4}}, request.Limits)
}

func (service *Service) buildDirectReferences(
	ctx context.Context,
	request Request,
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
	canonical map[string]repositoryfacts.Symbol,
	pack *EvidencePack,
) error {
	target, err := service.resolveExactSymbol(ctx, request, canonical)
	if err != nil {
		return err
	}
	edges, err := service.boundedNeighborhood(ctx, request, analysis, []string{target.ID})
	if err != nil {
		return err
	}
	incoming := make([]repositoryfacts.Edge, 0, len(edges))
	callers := make(map[string]repositoryfacts.Symbol)
	for _, edge := range edges {
		caller, isSymbol := canonical[edge.FromID]
		if edge.ToID != target.ID || !isSymbol {
			continue
		}
		incoming = append(incoming, edge)
		callers[caller.ID] = caller
	}
	callerIDs := make([]string, 0, len(callers))
	for id := range callers {
		callerIDs = append(callerIDs, id)
	}
	sort.Strings(callerIDs)
	selected := []scoredSymbol{{symbol: target, score: 4}}
	for _, id := range callerIDs {
		if len(selected) == request.Limits.MaxSymbols {
			if err := addOmittedSymbol(pack, id); err != nil {
				return err
			}
			continue
		}
		selected = append(selected, scoredSymbol{symbol: callers[id]})
	}
	pack.SubjectSymbolID = target.ID
	if err := addEvidenceSymbols(pack, snapshot, selected, request.Limits); err != nil {
		return err
	}
	addEvidenceRelations(pack, incoming, scoredSymbolIDs(selected), target.ID)
	return nil
}
