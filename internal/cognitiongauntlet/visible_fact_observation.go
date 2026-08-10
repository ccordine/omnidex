package cognitiongauntlet

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type visibleSymbolicObservation struct {
	Format           string                `json:"format"`
	Predicates       []cognition.Predicate `json:"predicates"`
	Records          []visibleRecordWire   `json:"records,omitempty"`
	RecordsTruncated bool                  `json:"records_truncated,omitempty"`
	GoalSatisfied    bool                  `json:"goal_satisfied"`
}

type visibleRecordWire struct {
	ID            string `json:"id"`
	Location      string `json:"location"`
	Content       string `json:"content,omitempty"`
	ContentSHA256 string `json:"content_sha256"`
}

type visibleSurfaceObservation struct {
	Surface          string          `json:"surface"`
	Operation        string          `json:"operation"`
	SymbolicState    json.RawMessage `json:"symbolic_state"`
	SurfaceAuthority string          `json:"surface_authority"`
	Result           json.RawMessage `json:"result"`
}

func visibleFactRecords(content string) ([]visibleFactRecord, error) {
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(content), &fields); err != nil {
		return nil, fmt.Errorf("decode public observation discriminator: %w", err)
	}
	if _, symbolic := fields["format"]; symbolic {
		return visibleSymbolicFactRecords([]byte(content))
	}
	if _, surface := fields["surface"]; surface {
		return visibleSurfaceFactRecords([]byte(content))
	}
	return nil, fmt.Errorf("public observation format is not registered")
}

func visibleSymbolicFactRecords(raw []byte) ([]visibleFactRecord, error) {
	payload := visibleSymbolicObservation{}
	if err := decodeStrictJSON(raw, &payload, "symbolic public fact observation"); err != nil {
		return nil, err
	}
	if payload.Format != "symbolic-observation.v1" || payload.Predicates == nil {
		return nil, fmt.Errorf("symbolic public observation authority is invalid")
	}
	for index, predicate := range payload.Predicates {
		if err := predicate.Validate(); err != nil {
			return nil, fmt.Errorf("symbolic public predicate %d: %w", index+1, err)
		}
	}
	return collectVisibleFactRecords(payload.Records)
}

func visibleSurfaceFactRecords(raw []byte) ([]visibleFactRecord, error) {
	payload := visibleSurfaceObservation{}
	if err := decodeStrictJSON(raw, &payload, "surface public fact observation"); err != nil {
		return nil, err
	}
	state := visibleSymbolicObservation{}
	if err := decodeStrictJSON(payload.SymbolicState, &state, "surface symbolic public state"); err != nil {
		return nil, err
	}
	if state.Format != "symbolic-observation.v1" || state.Predicates == nil ||
		!validDigest(payload.SurfaceAuthority) {
		return nil, fmt.Errorf("surface public observation authority is invalid")
	}
	for index, predicate := range state.Predicates {
		if err := predicate.Validate(); err != nil {
			return nil, fmt.Errorf("surface public predicate %d: %w", index+1, err)
		}
	}
	switch payload.Surface {
	case "filesystem.v1":
		return visibleFilesystemFactRecords(payload.Operation, payload.Result)
	case "record-surface.v1":
		return visibleRecordSurfaceFactRecords(payload.Operation, payload.Result)
	default:
		return nil, fmt.Errorf("surface public observation version is not registered")
	}
}

func collectVisibleFactRecords(records []visibleRecordWire) ([]visibleFactRecord, error) {
	result := make(map[string]visibleFactRecord)
	for _, record := range records {
		if requireExact(record.ID, "public record ID", 256) != nil ||
			requireExact(record.Location, "public record location", 256) != nil ||
			!validDigest(record.ContentSHA256) {
			return nil, fmt.Errorf("public record identity, location, or digest is invalid")
		}
		if record.Content == "" {
			continue
		}
		if visibleFactDigest(record.Content) != record.ContentSHA256 {
			return nil, fmt.Errorf("public record content hash is invalid")
		}
		if _, duplicate := result[record.ID]; duplicate {
			return nil, fmt.Errorf("public record identity is duplicated")
		}
		result[record.ID] = visibleFactRecord{
			ID: record.ID, Content: record.Content, ContentSHA256: record.ContentSHA256,
		}
	}
	return canonicalVisibleFactRecords(result)
}
