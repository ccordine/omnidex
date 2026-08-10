package labyrinth

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func (surface *filesystemSurface) Start(
	ctx context.Context,
	scenario Scenario,
) (surfacePreparation, error) {
	if surface.closed {
		return surfacePreparation{}, ErrSurfaceClosed
	}
	stage, err := currentStage(scenario)
	if err != nil {
		return surfacePreparation{}, err
	}
	state := newFilesystemState(surface.records, stage)
	result, err := surface.execute(ctx, &state, "observe", stage, stage, cognition.ActionRequest{})
	if err != nil {
		return surfacePreparation{}, err
	}
	return newSurfacePreparation(FilesystemSurfaceVersionV1, "observe", state, result)
}

func (surface *filesystemSurface) Apply(
	ctx context.Context,
	_ Scenario,
	current surfacePreparation,
	action cognition.RegisteredAction,
) (surfacePreparation, error) {
	if surface.closed {
		return surfacePreparation{}, ErrSurfaceClosed
	}
	if current.Version != FilesystemSurfaceVersionV1 {
		return surfacePreparation{}, fmt.Errorf("%w: current surface version is invalid", ErrSurfaceOperation)
	}
	state, err := decodeFilesystemState(current)
	if err != nil {
		return surfacePreparation{}, err
	}
	currentLocation, destination := state.Current, state.Current
	if action.Request.Kind == "navigate" {
		from, to, endpointErr := filesystemNavigationEndpoints(action)
		if endpointErr != nil {
			return surfacePreparation{}, endpointErr
		}
		if from != state.Current {
			return surfacePreparation{}, ErrSurfacePrecondition
		}
		currentLocation, destination = from, to
	}
	result, err := surface.execute(
		ctx, &state, string(action.Request.Kind), currentLocation, destination, action.Request,
	)
	if err != nil {
		return surfacePreparation{}, err
	}
	if action.Request.Kind == "navigate" {
		state.Current = destination
	}
	state.canonicalize()
	if err := state.Validate(); err != nil {
		return surfacePreparation{}, err
	}
	return newSurfacePreparation(FilesystemSurfaceVersionV1, string(action.Request.Kind), state, result)
}

func (surface *filesystemSurface) execute(
	ctx context.Context,
	state *filesystemState,
	operation string,
	from, to EntityID,
	request cognition.ActionRequest,
) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !surface.hasStage(from) || !surface.hasStage(to) {
		return nil, fmt.Errorf("%w: action stage is absent", ErrSurfaceOperation)
	}
	surface.executions++
	root, cleanup, err := surface.materialize(state)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	switch cognition.ActionKind(operation) {
	case "observe":
		return surface.observe(root, state, from)
	case "search":
		query, err := requiredExactSurfaceArgument(request, queryArg)
		if err != nil {
			return nil, err
		}
		return surface.search(ctx, root, state, query)
	case "read":
		artifact, err := requiredSurfaceArgument(request, artifactArg)
		if err != nil {
			return nil, err
		}
		return surface.read(root, state, EntityID(artifact))
	case "navigate":
		return surface.navigate(root, from, to)
	case "take":
		object, err := requiredSurfaceArgument(request, objectArg)
		if err != nil {
			return nil, err
		}
		return surface.take(root, state, from, EntityID(object))
	case "use":
		item, err := requiredSurfaceArgument(request, itemArg)
		if err != nil {
			return nil, err
		}
		target, err := requiredSurfaceArgument(request, targetArg)
		if err != nil {
			return nil, err
		}
		return surface.use(state, EntityID(item), EntityID(target), from)
	case "write":
		target, err := requiredSurfaceArgument(request, mutationTargetArg)
		if err != nil {
			return nil, err
		}
		value, err := requiredSurfaceArgument(request, mutationValueArg)
		if err != nil {
			return nil, err
		}
		expected, err := requiredSurfaceArgument(request, expectedSHA256Arg)
		if err != nil || !validDigest(expected) {
			return nil, ErrSurfacePrecondition
		}
		return surface.write(root, state, from, EntityID(target), expected, value)
	default:
		return nil, fmt.Errorf("%w: operation %q is not registered", ErrSurfaceOperation, operation)
	}
}

func (surface *filesystemSurface) hasStage(stage EntityID) bool {
	for _, candidate := range surface.stages {
		if candidate == stage {
			return true
		}
	}
	return false
}
