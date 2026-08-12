package repositoryobjective

import (
	"context"
	"fmt"
	"sort"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

const maxDirectRelations = 64

type relationEvidence struct {
	calls   []SymbolEvidence
	callers []SymbolEvidence
	tests   []SymbolEvidence
}

func inspectDirectRelations(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	subjectID string,
) (relationEvidence, error) {
	symbols := make(map[string]repositoryfacts.Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	calls := make(map[string]repositoryfacts.Symbol)
	callers := make(map[string]repositoryfacts.Symbol)
	tests := make(map[string]repositoryfacts.Symbol)
	for _, edge := range analysis.Edges {
		switch {
		case edge.Kind == "calls" && edge.FromID == subjectID:
			if symbol, exists := symbols[edge.ToID]; exists {
				calls[symbol.ID] = symbol
			}
		case edge.Kind == "calls" && edge.ToID == subjectID:
			if symbol, exists := symbols[edge.FromID]; exists {
				callers[symbol.ID] = symbol
			}
		case edge.Kind == "tests" && edge.ToID == subjectID:
			if symbol, exists := symbols[edge.FromID]; exists && symbol.Kind == "test" {
				tests[symbol.ID] = symbol
			}
		}
	}
	total := len(calls) + len(callers) + len(tests)
	if total > maxDirectRelations {
		return relationEvidence{}, fmt.Errorf(
			"%w: subject %q has %d direct relations; maximum is %d",
			ErrRelationBound, subjectID, total, maxDirectRelations,
		)
	}
	observed := make(map[string]SymbolEvidence, total)
	for _, group := range []map[string]repositoryfacts.Symbol{calls, callers, tests} {
		for id, symbol := range group {
			if _, exists := observed[id]; exists {
				continue
			}
			if err := ctx.Err(); err != nil {
				return relationEvidence{}, err
			}
			span, err := repositoryfacts.ReadExactSymbolSpan(snapshot, symbol, maxDeclarationBytes)
			if err != nil {
				return relationEvidence{}, fmt.Errorf("inspect direct relation %q: %w", id, err)
			}
			observed[id] = symbolEvidence(symbol, span)
		}
	}
	return relationEvidence{
		calls: evidenceForIDs(calls, observed), callers: evidenceForIDs(callers, observed),
		tests: evidenceForIDs(tests, observed),
	}, nil
}

func evidenceForIDs(
	symbols map[string]repositoryfacts.Symbol,
	observed map[string]SymbolEvidence,
) []SymbolEvidence {
	result := make([]SymbolEvidence, 0, len(symbols))
	for id := range symbols {
		result = append(result, observed[id])
	}
	sort.Slice(result, func(left, right int) bool { return result[left].SymbolID < result[right].SymbolID })
	return result
}
