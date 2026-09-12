package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/jackc/pgx/v5"
)

func databaseCitationPosition(metadata map[string]any) (int64, int, int, error) {
	values := [3]int64{}
	for index, key := range []string{"database_evidence_id", "database_row_start", "database_row_end"} {
		text, err := objectiveMetadataString(metadata, key, 20)
		if err != nil {
			return 0, 0, 0, err
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil || value < 0 || strconv.FormatInt(value, 10) != text {
			return 0, 0, 0, fmt.Errorf("database citation %q must be a canonical nonnegative integer", key)
		}
		values[index] = value
	}
	if values[0] < 1 || values[1] > values[2] || values[2] > datasource.MaxIntentRows {
		return 0, 0, 0, fmt.Errorf("database citation requires a persisted execution ID and bounded row range")
	}
	return values[0], int(values[1]), int(values[2]), nil
}

func validateRecordedDatabaseCitation(ctx context.Context, tx pgx.Tx, citation evidence.Record) error {
	id, start, end, err := databaseCitationPosition(citation.Metadata)
	if err != nil {
		return err
	}
	record, err := readDatabaseEvidence(ctx, tx, citation.JobID, id)
	if err != nil {
		return err
	}
	if citation.SourceRef != record.SourceRef() ||
		citation.Metadata["source_acquired_at"] != record.Evidence.Execution.AcquiredAt.Format(time.RFC3339Nano) {
		return fmt.Errorf("database citation source or acquisition time differs from its recorded execution")
	}
	projection, err := datasource.ProjectEvidenceRows(record.Snapshot, record.Plan.Intent, record.Evidence.Result, start, end)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return fmt.Errorf("encode recorded database result projection: %w", err)
	}
	if citation.Excerpt != string(encoded) {
		return fmt.Errorf("database citation excerpt differs from the recorded query rows")
	}
	return nil
}
