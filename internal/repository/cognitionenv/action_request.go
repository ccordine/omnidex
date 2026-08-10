package cognitionenv

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func (environment *Environment) retrievalRequest(
	state investigationState,
	action cognition.RegisteredAction,
) (repositoryretrieval.Request, error) {
	request := repositoryretrieval.Request{
		ProjectID:  environment.investigation.projectID,
		AnalysisID: environment.investigation.analysis.ID,
		Limits:     fixedLimits,
	}
	switch action.Request.Kind {
	case ActionSearch:
		if state.Stage != stageStart {
			return repositoryretrieval.Request{}, fmt.Errorf("repository search is only valid at investigation start")
		}
		request.Operation = repositoryretrieval.OperationSemanticExcerpts
		request.Query = environment.investigation.query
		return request, nil
	case ActionInspect:
		if state.Stage != stageSearched && state.Stage != stageInspected {
			return repositoryretrieval.Request{}, fmt.Errorf("repository inspect requires prior discovery")
		}
		ref, err := symbolArgument(action)
		if err != nil || !slices.Contains(state.DiscoveredRefs, ref) {
			return repositoryretrieval.Request{}, fmt.Errorf("repository inspect target was not discovered")
		}
		symbol, exists := environment.symbol(ref)
		if !exists {
			return repositoryretrieval.Request{}, fmt.Errorf("repository inspect target is absent from exact analysis")
		}
		if !environment.matchesAcceptedSubject(symbol) {
			return repositoryretrieval.Request{}, fmt.Errorf(
				"repository inspect target does not match the registered query authority",
			)
		}
		request.Operation = repositoryretrieval.OperationSymbolDeclaration
		request.Query = symbol.QualifiedName
		return request, nil
	case ActionReferences:
		if state.Stage != stageInspected {
			return repositoryretrieval.Request{}, fmt.Errorf("repository references require prior inspection")
		}
		ref, err := symbolArgument(action)
		if err != nil || !slices.Contains(state.InspectedRefs, ref) {
			return repositoryretrieval.Request{}, fmt.Errorf("repository reference target was not inspected")
		}
		symbol, exists := environment.symbol(ref)
		if !exists {
			return repositoryretrieval.Request{}, fmt.Errorf("repository reference target is absent from exact analysis")
		}
		if !environment.matchesAcceptedSubject(symbol) {
			return repositoryretrieval.Request{}, fmt.Errorf(
				"repository reference target does not match the registered query authority",
			)
		}
		request.Operation = repositoryretrieval.OperationDirectReferences
		request.Query = symbol.QualifiedName
		return request, nil
	default:
		return repositoryretrieval.Request{}, fmt.Errorf("repository investigation action is not registered")
	}
}

func (environment *Environment) matchesAcceptedSubject(symbol repositoryfacts.Symbol) bool {
	query := environment.investigation.query
	return strings.EqualFold(symbol.Name, query) || strings.EqualFold(symbol.QualifiedName, query)
}
