package labyrinth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const (
	MaxSurfaceStateBytes  = 1024 * 1024
	MaxSurfaceResultBytes = 16 * 1024
)

type surfacePreparation struct {
	Version     string
	State       json.RawMessage
	StateSHA256 string
	Operation   string
	Result      json.RawMessage
}

func newSurfacePreparation(version, operation string, state, result any) (surfacePreparation, error) {
	_, stateRaw, err := digestJSON(state)
	if err != nil {
		return surfacePreparation{}, fmt.Errorf("%w: encode state: %v", ErrSurfaceOperation, err)
	}
	resultRaw, err := json.Marshal(result)
	if err != nil {
		return surfacePreparation{}, fmt.Errorf("%w: encode result: %v", ErrSurfaceOperation, err)
	}
	preparation := surfacePreparation{
		Version: version, State: stateRaw,
		Operation: operation, Result: resultRaw,
	}
	preparation.StateSHA256, _, err = digestJSON(struct {
		Version string          `json:"version"`
		State   json.RawMessage `json:"state"`
	}{version, stateRaw})
	if err != nil {
		return surfacePreparation{}, fmt.Errorf("%w: bind state authority: %v", ErrSurfaceOperation, err)
	}
	if err := preparation.Validate(); err != nil {
		return surfacePreparation{}, err
	}
	return preparation, nil
}

func (preparation surfacePreparation) Validate() error {
	if !validSymbol(preparation.Version) || !validSymbol(preparation.Operation) {
		return fmt.Errorf("%w: surface version or operation is invalid", ErrSurfaceOperation)
	}
	stateSHA, _, err := digestJSON(struct {
		Version string          `json:"version"`
		State   json.RawMessage `json:"state"`
	}{preparation.Version, preparation.State})
	if len(preparation.State) == 0 || len(preparation.State) > MaxSurfaceStateBytes ||
		!json.Valid(preparation.State) || err != nil || stateSHA != preparation.StateSHA256 {
		return fmt.Errorf("%w: surface state authority is invalid", ErrSurfaceLimit)
	}
	if len(preparation.Result) == 0 || len(preparation.Result) > MaxSurfaceResultBytes || !json.Valid(preparation.Result) {
		return fmt.Errorf("%w: surface result is invalid or exceeds %d bytes", ErrSurfaceLimit, MaxSurfaceResultBytes)
	}
	return nil
}

func (preparation surfacePreparation) clone() surfacePreparation {
	preparation.State = append(json.RawMessage(nil), preparation.State...)
	preparation.Result = append(json.RawMessage(nil), preparation.Result...)
	return preparation
}

type environmentSurface interface {
	Start(context.Context, Scenario) (surfacePreparation, error)
	Apply(context.Context, Scenario, surfacePreparation, cognition.RegisteredAction) (surfacePreparation, error)
	Close() error
}

func buildSurfaceObservation(
	actionID cognition.ActionID,
	revision cognition.WorldRevision,
	facts factSet,
	terminal bool,
	entities map[EntityID]Entity,
	schemas map[cognition.PredicateName]PredicateSchema,
	preparation surfacePreparation,
) (cognition.Observation, error) {
	state, err := publicObservationContent(facts, terminal, entities, schemas, nil, false)
	if err != nil {
		return cognition.Observation{}, err
	}
	payload := struct {
		Surface          string          `json:"surface"`
		Operation        string          `json:"operation"`
		SymbolicState    json.RawMessage `json:"symbolic_state"`
		SurfaceAuthority string          `json:"surface_authority"`
		Result           json.RawMessage `json:"result"`
	}{preparation.Version, preparation.Operation, json.RawMessage(state), preparation.StateSHA256, preparation.Result}
	raw, err := json.Marshal(payload)
	if err != nil {
		return cognition.Observation{}, fmt.Errorf("%w: encode public surface result: %v", ErrSurfaceOperation, err)
	}
	if len(raw) > cognition.MaxObservationBytes {
		return cognition.Observation{}, fmt.Errorf("%w: public surface observation exceeds %d bytes", ErrSurfaceLimit, cognition.MaxObservationBytes)
	}
	return newObservationFromContent(actionID, revision, string(raw))
}
