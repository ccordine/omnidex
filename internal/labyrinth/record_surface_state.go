package labyrinth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

const (
	recordSurfaceStateFormatV1  = "record-state.v1"
	recordSurfaceResultFormatV1 = "record-result.v1"
	maxRecordSurfaceTextBytes   = 1024
)

type recordSurface struct {
	mu     sync.Mutex
	closed bool
}

type recordSurfaceState struct {
	Format  string                `json:"format"`
	Current EntityID              `json:"current"`
	Held    EntityID              `json:"held,omitempty"`
	Used    EntityID              `json:"used,omitempty"`
	Records []recordSurfaceRecord `json:"records"`
}

type recordSurfaceRecord struct {
	ID         EntityID `json:"id"`
	Collection EntityID `json:"collection"`
	Content    string   `json:"content"`
	SHA256     string   `json:"sha256"`
}

func newRecordSurfaceState(scenario Scenario) (recordSurfaceState, error) {
	current, err := initialRecordCollection(scenario)
	if err != nil {
		return recordSurfaceState{}, err
	}
	records := make([]recordSurfaceRecord, len(scenario.descriptor.Records))
	for index, record := range scenario.descriptor.Records {
		records[index] = recordSurfaceRecord{record.ID, record.Location, record.Content, record.ContentSHA256}
	}
	state := recordSurfaceState{recordSurfaceStateFormatV1, current, "", "", records}
	if err := state.Validate(scenario); err != nil {
		return recordSurfaceState{}, err
	}
	return state, nil
}

func decodeRecordSurfaceState(
	scenario Scenario,
	preparation surfacePreparation,
) (recordSurfaceState, error) {
	if err := preparation.Validate(); err != nil {
		return recordSurfaceState{}, err
	}
	if preparation.Version != RecordSurfaceVersionV1 {
		return recordSurfaceState{}, fmt.Errorf("%w: prior surface version is not record v1", ErrSurfaceOperation)
	}
	var state recordSurfaceState
	if err := json.Unmarshal(preparation.State, &state); err != nil {
		return recordSurfaceState{}, fmt.Errorf("%w: decode prior record state: %v", ErrSurfaceOperation, err)
	}
	if err := state.Validate(scenario); err != nil {
		return recordSurfaceState{}, err
	}
	_, canonical, err := digestJSON(state)
	if err != nil || !bytes.Equal(canonical, preparation.State) {
		return recordSurfaceState{}, fmt.Errorf("%w: prior record state is not canonical", ErrSurfaceOperation)
	}
	return state, nil
}

func (state recordSurfaceState) Validate(scenario Scenario) error {
	if state.Format != recordSurfaceStateFormatV1 {
		return fmt.Errorf("%w: record state format is invalid", ErrSurfaceOperation)
	}
	collections := publicRecordCollections(scenario)
	known := make(map[EntityID]struct{}, len(collections))
	for _, collection := range collections {
		known[collection] = struct{}{}
	}
	if _, exists := known[state.Current]; !exists {
		return fmt.Errorf("%w: current record collection is unknown", ErrSurfaceOperation)
	}
	recordIDs := make(map[EntityID]struct{}, len(state.Records))
	if len(state.Records) != len(scenario.descriptor.Records) {
		return fmt.Errorf("%w: record inventory is incomplete", ErrSurfaceOperation)
	}
	for index, record := range state.Records {
		declared := scenario.descriptor.Records[index]
		if record.ID != declared.ID || record.Collection != declared.Location || record.Content == "" ||
			len(record.Content) > maxRecordSurfaceTextBytes || !utf8.ValidString(record.Content) ||
			textSHA256(record.Content) != record.SHA256 {
			return fmt.Errorf("%w: record %d is invalid", ErrSurfaceOperation, index)
		}
		recordIDs[record.ID] = struct{}{}
	}
	for _, optional := range []EntityID{state.Held, state.Used} {
		if optional != "" {
			if _, exists := recordIDs[optional]; !exists {
				return fmt.Errorf("%w: retained record is unknown", ErrSurfaceOperation)
			}
		}
	}
	return nil
}

func publicRecordCollections(scenario Scenario) []EntityID {
	collections := make([]EntityID, 0)
	for _, entity := range scenario.definition.entities {
		if entity.Public && entity.Kind == stageKind {
			collections = append(collections, entity.ID)
		}
	}
	return collections
}

func initialRecordCollection(scenario Scenario) (EntityID, error) {
	var current EntityID
	for _, fact := range scenario.definition.initialFacts {
		if fact.Name != "state.current" {
			continue
		}
		if len(fact.Args) != 1 || current != "" {
			return "", fmt.Errorf("%w: record surface requires exactly one initial collection", ErrSurfaceOperation)
		}
		current = EntityID(fact.Args[0])
	}
	if current == "" {
		return "", fmt.Errorf("%w: record surface has no initial collection", ErrSurfaceOperation)
	}
	return current, nil
}

func (surface *recordSurface) Close() error {
	surface.mu.Lock()
	defer surface.mu.Unlock()
	surface.closed = true
	return nil
}

func (surface *recordSurface) requireOpen() error {
	if surface.closed {
		return ErrSurfaceClosed
	}
	return nil
}

var _ environmentSurface = (*recordSurface)(nil)
var _ cognition.Environment = (*RecordEnvironment)(nil)
