package labyrinth

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
)

const (
	maxRecordSurfaceEntries     = 16
	maxRecordSurfaceReadEntries = 8
)

type recordSurfaceResult struct {
	Format              string               `json:"format"`
	Collection          EntityID             `json:"collection"`
	PreviousCollection  EntityID             `json:"previous_collection,omitempty"`
	Query               string               `json:"query,omitempty"`
	Records             []recordSurfaceEntry `json:"records"`
	TotalRecords        int                  `json:"total_records"`
	OmittedRecords      int                  `json:"omitted_records"`
	HeldRecord          EntityID             `json:"held_record,omitempty"`
	UsedRecord          EntityID             `json:"used_record,omitempty"`
	CurrentRecordSHA256 string               `json:"current_record_sha256"`
	PreviousSHA256      string               `json:"previous_sha256,omitempty"`
	CurrentSHA256       string               `json:"current_sha256,omitempty"`
}

type recordSurfaceEntry struct {
	ID            EntityID `json:"id"`
	Location      EntityID `json:"location"`
	Content       string   `json:"content,omitempty"`
	ContentSHA256 string   `json:"content_sha256"`
}

func buildRecordSurfaceResult(
	state recordSurfaceState,
	operation string,
	request cognition.ActionRequest,
	previous EntityID,
	previousSHA string,
	currentSHA string,
) (recordSurfaceResult, error) {
	records, contentMode, query, err := selectedRecordSurfaceRecords(state, operation, request)
	if err != nil {
		return recordSurfaceResult{}, err
	}
	total := len(records)
	limit := maxRecordSurfaceEntries
	if contentMode {
		limit = maxRecordSurfaceReadEntries
	}
	if len(records) > limit {
		records = records[:limit]
	}
	entries := make([]recordSurfaceEntry, len(records))
	for index, record := range records {
		content := ""
		if contentMode {
			content = record.Content
		}
		entries[index] = recordSurfaceEntry{
			record.ID, record.Collection, content, record.SHA256,
		}
	}
	recordSHA := ""
	if target := actionArgument(request, mutationTargetArg); target != "" {
		if record := recordByID(&state, EntityID(target)); record != nil {
			recordSHA = record.SHA256
		}
	}
	return recordSurfaceResult{
		Format: recordSurfaceResultFormatV1, Collection: state.Current,
		PreviousCollection: previous, Query: query, Records: entries, TotalRecords: total,
		OmittedRecords: total - len(entries), HeldRecord: state.Held, UsedRecord: state.Used,
		CurrentRecordSHA256: recordSHA, PreviousSHA256: previousSHA, CurrentSHA256: currentSHA,
	}, nil
}

func selectedRecordSurfaceRecords(
	state recordSurfaceState,
	operation string,
	request cognition.ActionRequest,
) ([]recordSurfaceRecord, bool, string, error) {
	switch cognition.ActionKind(operation) {
	case "search":
		query, err := requiredExactSurfaceArgument(request, queryArg)
		if err != nil {
			return nil, false, "", err
		}
		matches := make([]recordSurfaceRecord, 0)
		for _, record := range state.Records {
			if string(record.ID) == query || strings.Contains(record.Content, query) {
				matches = append(matches, record)
			}
		}
		return matches, true, query, nil
	case "read":
		artifact, err := requiredSurfaceArgument(request, artifactArg)
		if err != nil {
			return nil, false, "", err
		}
		record := recordByID(&state, EntityID(artifact))
		if record == nil {
			return nil, false, "", ErrSurfacePrecondition
		}
		return []recordSurfaceRecord{*record}, true, "", nil
	case "observe":
		return recordsAtCollection(state, state.Current), false, "", nil
	case "navigate", "take", "use", "write":
		return []recordSurfaceRecord{}, false, "", nil
	default:
		return nil, false, "", fmt.Errorf("%w: record result operation is unregistered", ErrSurfaceOperation)
	}
}

func recordsAtCollection(state recordSurfaceState, collection EntityID) []recordSurfaceRecord {
	result := make([]recordSurfaceRecord, 0)
	for _, record := range state.Records {
		if record.Collection == collection {
			result = append(result, record)
		}
	}
	return result
}
