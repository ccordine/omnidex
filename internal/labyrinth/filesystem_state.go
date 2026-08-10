package labyrinth

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

type filesystemDocument struct {
	ID            EntityID `json:"id"`
	Location      EntityID `json:"location"`
	Content       string   `json:"content"`
	ContentSHA256 string   `json:"content_sha256"`
}

type filesystemState struct {
	Current   EntityID             `json:"current"`
	Documents []filesystemDocument `json:"documents"`
	Inventory []EntityID           `json:"inventory"`
	Used      []EntityID           `json:"used"`
}

func newFilesystemState(records []PublicRecord, current EntityID) filesystemState {
	state := filesystemState{Current: current, Documents: make([]filesystemDocument, len(records))}
	for index, record := range records {
		state.Documents[index] = filesystemDocument{
			record.ID, record.Location, record.Content, record.ContentSHA256,
		}
	}
	state.canonicalize()
	return state
}

func decodeFilesystemState(preparation surfacePreparation) (filesystemState, error) {
	if err := preparation.Validate(); err != nil {
		return filesystemState{}, err
	}
	var state filesystemState
	if err := json.Unmarshal(preparation.State, &state); err != nil {
		return filesystemState{}, fmt.Errorf("%w: decode state: %v", ErrSurfaceOperation, err)
	}
	if err := state.Validate(); err != nil {
		return filesystemState{}, err
	}
	return state, nil
}

func (state *filesystemState) canonicalize() {
	sort.Slice(state.Documents, func(left, right int) bool { return state.Documents[left].ID < state.Documents[right].ID })
	sort.Slice(state.Inventory, func(left, right int) bool { return state.Inventory[left] < state.Inventory[right] })
	sort.Slice(state.Used, func(left, right int) bool { return state.Used[left] < state.Used[right] })
}

func (state filesystemState) Validate() error {
	if !safeSurfaceSegment(string(state.Current)) {
		return fmt.Errorf("%w: filesystem current stage is invalid", ErrSurfaceOperation)
	}
	previous := EntityID("")
	for index, document := range state.Documents {
		record := PublicRecord{document.ID, document.Location, document.Content, document.ContentSHA256}
		if record.Validate() != nil || !safeSurfaceSegment(string(document.ID)) ||
			!safeSurfaceSegment(string(document.Location)) || index > 0 && document.ID <= previous {
			return fmt.Errorf("%w: filesystem document state is invalid", ErrSurfaceOperation)
		}
		previous = document.ID
	}
	if err := validateSortedSurfaceIDs(state.Inventory, state.Documents); err != nil {
		return err
	}
	if err := validateSortedSurfaceIDs(state.Used, state.Documents); err != nil {
		return err
	}
	for _, id := range state.Used {
		if !containsEntityID(state.Inventory, id) {
			return fmt.Errorf("%w: used document is absent from inventory", ErrSurfaceOperation)
		}
	}
	return nil
}

func validateSortedSurfaceIDs(values []EntityID, documents []filesystemDocument) error {
	known := make(map[EntityID]struct{}, len(documents))
	for _, document := range documents {
		known[document.ID] = struct{}{}
	}
	for index, value := range values {
		if _, exists := known[value]; !exists || index > 0 && value <= values[index-1] {
			return fmt.Errorf("%w: filesystem collection is invalid", ErrSurfaceOperation)
		}
	}
	return nil
}

func filesystemNavigationEndpoints(action cognition.RegisteredAction) (EntityID, EntityID, error) {
	if action.Request.Kind != "navigate" {
		return "", "", fmt.Errorf("%w: only navigate has movement endpoints", ErrSurfaceOperation)
	}
	values := make(map[cognition.ActionArgumentName]string, len(action.Request.Arguments))
	for _, argument := range action.Request.Arguments {
		values[argument.Name] = argument.Value
	}
	from, to := EntityID(values[fromArg]), EntityID(values[toArg])
	if !safeSurfaceSegment(string(from)) || !safeSurfaceSegment(string(to)) {
		return "", "", fmt.Errorf("%w: action endpoint is unsafe", ErrSurfaceOperation)
	}
	return from, to, nil
}

func currentStage(scenario Scenario) (EntityID, error) {
	for _, fact := range scenario.definition.initialFacts {
		if fact.Name == "state.current" && len(fact.Args) == 1 {
			return EntityID(fact.Args[0]), nil
		}
	}
	return "", fmt.Errorf("%w: initial stage is absent", ErrSurfaceOperation)
}
