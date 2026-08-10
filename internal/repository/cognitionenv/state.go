package cognitionenv

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

const investigationStateSchemaV1 = "omnidex.repository-investigation-state.v1"

func initialInvestigationState() investigationState {
	return investigationState{
		Schema: investigationStateSchemaV1, Stage: stageStart,
		DiscoveredRefs: []string{}, InspectedRefs: []string{},
	}
}

func (state investigationState) Validate(investigation Investigation) error {
	if state.Schema != investigationStateSchemaV1 || state.DiscoveredRefs == nil ||
		state.InspectedRefs == nil {
		return fmt.Errorf("repository investigation state is not canonical")
	}
	switch state.Stage {
	case stageStart:
		if len(state.DiscoveredRefs) != 0 || len(state.InspectedRefs) != 0 ||
			state.EvidencePackID != "" {
			return fmt.Errorf("repository investigation start state contains acquired evidence")
		}
	case stageSearched, stageInspected, stageComplete:
		if len(state.DiscoveredRefs) == 0 || state.EvidencePackID == "" {
			return fmt.Errorf("repository investigation progress has no bounded evidence")
		}
	default:
		return fmt.Errorf("repository investigation stage %q is not registered", state.Stage)
	}
	if state.Stage == stageSearched && len(state.InspectedRefs) != 0 {
		return fmt.Errorf("repository search state claims inspected symbols")
	}
	if state.Stage == stageInspected && len(state.InspectedRefs) == 0 {
		return fmt.Errorf("repository inspect state has no inspected symbol")
	}
	if !sortedUnique(state.DiscoveredRefs) || !sortedUnique(state.InspectedRefs) {
		return fmt.Errorf("repository investigation symbol references are not uniquely sorted")
	}
	known := make(map[string]struct{}, len(investigation.analysis.Symbols))
	for _, symbol := range investigation.analysis.Symbols {
		known[symbol.ID] = struct{}{}
	}
	discovered := make(map[string]struct{}, len(state.DiscoveredRefs))
	for _, ref := range state.DiscoveredRefs {
		if _, exists := known[ref]; !exists {
			return fmt.Errorf("repository investigation state contains an unknown symbol reference")
		}
		discovered[ref] = struct{}{}
	}
	for _, ref := range state.InspectedRefs {
		if _, exists := discovered[ref]; !exists {
			return fmt.Errorf("repository investigation inspected an undiscovered symbol")
		}
	}
	return nil
}

func (environment *Environment) stateFromJournal(
	journal cognition.EnvironmentJournalState,
) (investigationState, error) {
	if journal.Current == journal.Start.Current {
		state := initialInvestigationState()
		return state, state.Validate(environment.investigation)
	}
	if journal.CurrentReceipt == nil || journal.CurrentReceipt.Transition == nil {
		return investigationState{}, fmt.Errorf("repository investigation progress has no durable receipt")
	}
	for _, observation := range journal.CurrentReceipt.Transition.Observations {
		if observation.Kind != ObservationState {
			continue
		}
		var state investigationState
		if err := json.Unmarshal([]byte(observation.Content), &state); err != nil {
			return investigationState{}, fmt.Errorf("decode repository investigation state: %w", err)
		}
		if err := state.Validate(environment.investigation); err != nil {
			return investigationState{}, err
		}
		return state, nil
	}
	return investigationState{}, fmt.Errorf("repository investigation receipt omitted its bounded state")
}

func (environment *Environment) terminalAction() cognition.ActionKind {
	switch environment.investigation.operation {
	case repositoryretrieval.OperationSemanticExcerpts:
		return ActionSearch
	case repositoryretrieval.OperationSymbolDeclaration:
		return ActionInspect
	case repositoryretrieval.OperationDirectReferences:
		return ActionReferences
	default:
		panic("validated repository investigation has unsupported operation")
	}
}

func (environment *Environment) symbol(ref string) (repositoryfacts.Symbol, bool) {
	for _, symbol := range environment.investigation.analysis.Symbols {
		if symbol.ID == ref {
			return symbol, true
		}
	}
	return repositoryfacts.Symbol{}, false
}

func symbolArgument(action cognition.RegisteredAction) (string, error) {
	for _, argument := range action.Request.Arguments {
		if argument.Name == ArgumentSymbolRef {
			return argument.Value, nil
		}
	}
	return "", fmt.Errorf("repository investigation action omitted symbol_ref")
}

func nextInvestigationState(
	prior investigationState,
	action cognition.RegisteredAction,
	pack repositoryretrieval.EvidencePack,
	terminal bool,
) (investigationState, error) {
	next := prior
	next.EvidencePackID = pack.ID
	switch action.Request.Kind {
	case ActionSearch:
		next.Stage = stageSearched
		next.DiscoveredRefs = make([]string, 0, len(pack.Symbols))
		for _, symbol := range pack.Symbols {
			next.DiscoveredRefs = append(next.DiscoveredRefs, symbol.ID)
		}
		next.InspectedRefs = []string{}
	case ActionInspect:
		next.Stage = stageInspected
		ref, err := symbolArgument(action)
		if err != nil {
			return investigationState{}, err
		}
		if !slices.Contains(next.InspectedRefs, ref) {
			next.InspectedRefs = append(next.InspectedRefs, ref)
		}
	case ActionReferences:
		next.Stage = stageComplete
	default:
		return investigationState{}, fmt.Errorf("repository investigation action is unsupported")
	}
	if terminal {
		next.Stage = stageComplete
	}
	sort.Strings(next.DiscoveredRefs)
	sort.Strings(next.InspectedRefs)
	return next, nil
}

func sortedUnique(values []string) bool {
	for index, value := range values {
		if value == "" || (index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}
