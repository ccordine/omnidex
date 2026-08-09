package retrieval

import (
	"context"
	"fmt"
	"slices"
	"sort"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (service *Service) boundedNeighborhood(
	ctx context.Context,
	request Request,
	analysis repositoryfacts.Analysis,
	subjectIDs []string,
) ([]repositoryfacts.Edge, error) {
	neighborhood, err := service.store.RepositoryGraphNeighborhood(
		ctx, request.ProjectID, request.AnalysisID, subjectIDs, request.Limits.MaxEdges+1,
	)
	if err != nil {
		return nil, err
	}
	if len(neighborhood.Edges) > request.Limits.MaxEdges {
		return nil, fmt.Errorf("repository graph exceeds the explicit %d-edge limit; narrow the query", request.Limits.MaxEdges)
	}
	if neighborhood.AnalysisID != analysis.ID {
		return nil, fmt.Errorf("repository graph returned analysis %q for %q", neighborhood.AnalysisID, analysis.ID)
	}
	wantSubjects := append([]string(nil), subjectIDs...)
	gotSubjects := append([]string(nil), neighborhood.SubjectIDs...)
	sort.Strings(wantSubjects)
	sort.Strings(gotSubjects)
	if !slices.Equal(gotSubjects, wantSubjects) {
		return nil, fmt.Errorf("repository graph returned the wrong subject projection")
	}
	canonical := make(map[string]repositoryfacts.Edge, len(analysis.Edges))
	for _, edge := range analysis.Edges {
		canonical[edge.ID] = edge
	}
	edges := make([]repositoryfacts.Edge, 0, len(neighborhood.Edges))
	seen := make(map[string]struct{}, len(neighborhood.Edges))
	for _, edge := range neighborhood.Edges {
		exact, exists := canonical[edge.ID]
		if !exists {
			return nil, fmt.Errorf("repository graph returned unknown edge %q", edge.ID)
		}
		if edge != exact {
			return nil, fmt.Errorf("repository graph returned altered edge %q", edge.ID)
		}
		if _, duplicate := seen[edge.ID]; duplicate {
			return nil, fmt.Errorf("repository graph returned duplicate edge %q", edge.ID)
		}
		seen[edge.ID] = struct{}{}
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(left, right int) bool { return edges[left].ID < edges[right].ID })
	return edges, nil
}
