package labyrinth

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

func (surface *recordSurface) Start(
	ctx context.Context,
	scenario Scenario,
) (surfacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return surfacePreparation{}, err
	}
	surface.mu.Lock()
	defer surface.mu.Unlock()
	if err := surface.requireOpen(); err != nil {
		return surfacePreparation{}, err
	}
	state, err := newRecordSurfaceState(scenario)
	if err != nil {
		return surfacePreparation{}, err
	}
	result, err := buildRecordSurfaceResult(state, "observe", cognition.ActionRequest{}, "", "", "")
	if err != nil {
		return surfacePreparation{}, err
	}
	return newSurfacePreparation(RecordSurfaceVersionV1, "observe", state, result)
}

func (surface *recordSurface) Apply(
	ctx context.Context,
	scenario Scenario,
	previous surfacePreparation,
	action cognition.RegisteredAction,
) (surfacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return surfacePreparation{}, err
	}
	surface.mu.Lock()
	defer surface.mu.Unlock()
	if err := surface.requireOpen(); err != nil {
		return surfacePreparation{}, err
	}
	state, err := decodeRecordSurfaceState(scenario, previous)
	if err != nil {
		return surfacePreparation{}, err
	}
	if !recordSurfaceKind(action.Request.Kind) {
		return surfacePreparation{}, fmt.Errorf("%w: record operation %q is not registered", ErrSurfaceOperation, action.Request.Kind)
	}
	previousSHA, currentSHA := "", ""
	previousCollection := state.Current
	switch action.Request.Kind {
	case "observe":
	case "search":
		if _, err := requiredExactSurfaceArgument(action.Request, queryArg); err != nil {
			return surfacePreparation{}, err
		}
	case "read":
		artifact, readErr := requiredSurfaceArgument(action.Request, artifactArg)
		if readErr != nil || recordByID(&state, EntityID(artifact)) == nil {
			return surfacePreparation{}, ErrSurfacePrecondition
		}
	case "navigate":
		from, to, endpointErr := recordNavigationEndpoints(action.Request)
		if endpointErr != nil {
			return surfacePreparation{}, endpointErr
		}
		if state.Current != from {
			return surfacePreparation{}, ErrSurfacePrecondition
		}
		state.Current = to
	case "take":
		object, objectErr := requiredSurfaceArgument(action.Request, objectArg)
		record := recordByID(&state, EntityID(object))
		if objectErr != nil || record == nil || record.Collection != state.Current || state.Held == record.ID {
			return surfacePreparation{}, ErrSurfacePrecondition
		}
		state.Held = record.ID
	case "use":
		item, itemErr := requiredSurfaceArgument(action.Request, itemArg)
		target, targetErr := requiredSurfaceArgument(action.Request, targetArg)
		if itemErr != nil || targetErr != nil || state.Held != EntityID(item) ||
			state.Current != EntityID(target) || state.Used == EntityID(item) {
			return surfacePreparation{}, ErrSurfacePrecondition
		}
		state.Used = EntityID(item)
	case "write":
		target, targetErr := requiredSurfaceArgument(action.Request, mutationTargetArg)
		if targetErr != nil {
			return surfacePreparation{}, targetErr
		}
		value, valueErr := requiredSurfaceArgument(action.Request, mutationValueArg)
		if valueErr != nil {
			return surfacePreparation{}, valueErr
		}
		expected, expectedErr := requiredSurfaceArgument(action.Request, expectedSHA256Arg)
		if expectedErr != nil || !validDigest(expected) {
			return surfacePreparation{}, ErrSurfacePrecondition
		}
		previousSHA, currentSHA, err = updateSurfaceRecord(
			&state, state.Current, EntityID(target), expected, value,
		)
		if err != nil {
			return surfacePreparation{}, err
		}
	}
	if err := state.Validate(scenario); err != nil {
		return surfacePreparation{}, err
	}
	operation := string(action.Request.Kind)
	result, err := buildRecordSurfaceResult(
		state, operation, action.Request, previousCollection, previousSHA, currentSHA,
	)
	if err != nil {
		return surfacePreparation{}, err
	}
	return newSurfacePreparation(RecordSurfaceVersionV1, operation, state, result)
}

func recordNavigationEndpoints(request cognition.ActionRequest) (EntityID, EntityID, error) {
	if request.Kind != "navigate" {
		return "", "", fmt.Errorf("%w: only navigate has collection endpoints", ErrSurfaceOperation)
	}
	values := make(map[cognition.ActionArgumentName]string, len(request.Arguments))
	for _, argument := range request.Arguments {
		values[argument.Name] = argument.Value
	}
	from, fromExists := values[fromArg]
	to, toExists := values[toArg]
	if !fromExists || !toExists || !validSymbol(from) || !validSymbol(to) {
		return "", "", fmt.Errorf("%w: record operation endpoints are invalid", ErrSurfaceOperation)
	}
	return EntityID(from), EntityID(to), nil
}

func recordSurfaceKind(kind cognition.ActionKind) bool {
	for _, registered := range v1MacroKinds {
		if kind == registered {
			return true
		}
	}
	return false
}

func updateSurfaceRecord(
	state *recordSurfaceState,
	collection EntityID,
	target EntityID,
	expected string,
	value string,
) (string, string, error) {
	record := recordByID(state, target)
	if record == nil || record.Collection != collection || record.SHA256 != expected || !validSymbol(value) {
		return "", "", ErrSurfacePrecondition
	}
	previous := record.SHA256
	record.Content = value
	record.SHA256 = textSHA256(record.Content)
	return previous, record.SHA256, nil
}

func recordByID(state *recordSurfaceState, id EntityID) *recordSurfaceRecord {
	for index := range state.Records {
		if state.Records[index].ID == id {
			return &state.Records[index]
		}
	}
	return nil
}

func requiredSurfaceArgument(
	request cognition.ActionRequest,
	name cognition.ActionArgumentName,
) (string, error) {
	for _, argument := range request.Arguments {
		if argument.Name == name && validSymbol(argument.Value) {
			return argument.Value, nil
		}
	}
	return "", fmt.Errorf("%w: action argument %q is required", ErrSurfaceOperation, name)
}

func requiredExactSurfaceArgument(
	request cognition.ActionRequest,
	name cognition.ActionArgumentName,
) (string, error) {
	for _, argument := range request.Arguments {
		if argument.Name == name && argument.Value != "" &&
			len(argument.Value) <= cognition.MaxActionValueBytes && utf8.ValidString(argument.Value) {
			return argument.Value, nil
		}
	}
	return "", fmt.Errorf("%w: exact action argument %q is required", ErrSurfaceOperation, name)
}

func firstRecordAt(state *recordSurfaceState, collection EntityID) *recordSurfaceRecord {
	for index := range state.Records {
		if state.Records[index].Collection == collection {
			return &state.Records[index]
		}
	}
	return nil
}
