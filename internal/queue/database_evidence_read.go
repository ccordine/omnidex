package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetDatabaseEvidence(ctx context.Context, jobID, id int64) (DatabaseEvidenceRecord, error) {
	if ctx == nil || r == nil || r.pool == nil || jobID < 1 || id < 1 {
		return DatabaseEvidenceRecord{}, fmt.Errorf("read database evidence requires PostgreSQL, context, and exact job and record IDs")
	}
	return readDatabaseEvidence(ctx, r.pool, jobID, id)
}

func readDatabaseEvidence(ctx context.Context, reader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, jobID, id int64) (DatabaseEvidenceRecord, error) {
	var record DatabaseEvidenceRecord
	var schemaJSON, planJSON, parametersJSON, resultJSON []byte
	execution := &record.Evidence.Execution
	var returnedRows, resultBytes int
	err := reader.QueryRow(ctx, `
		SELECT id, job_id, data_source_id, schema_snapshot, query_plan, query_text,
		       query_parameters, result_json, plan_total_cost, plan_estimated_rows,
		       returned_rows, result_bytes, acquired_at, duration_ms
		FROM database_evidence WHERE job_id=$1 AND id=$2
	`, jobID, id).Scan(&record.ID, &record.JobID, &execution.SourceID,
		&schemaJSON, &planJSON, &execution.Query.SQL, &parametersJSON, &resultJSON,
		&execution.Plan.TotalCost, &execution.Plan.EstimatedRows, &returnedRows,
		&resultBytes, &execution.AcquiredAt, &execution.DurationMS)
	if err != nil {
		return DatabaseEvidenceRecord{}, fmt.Errorf("read database execution %d for job %d: %w", id, jobID, err)
	}
	for _, field := range []struct {
		name string
		raw  []byte
		into any
	}{
		{"schema", schemaJSON, &record.Snapshot}, {"plan", planJSON, &record.Plan},
		{"parameters", parametersJSON, &execution.Query.Parameters}, {"result", resultJSON, &record.Evidence.Result},
	} {
		if err := json.Unmarshal(field.raw, field.into); err != nil {
			return DatabaseEvidenceRecord{}, fmt.Errorf("decode database evidence %s: %w", field.name, err)
		}
	}
	record.Evidence.Schema = datasource.EvidenceResultV1
	execution.AcquiredAt = execution.AcquiredAt.UTC()
	if record.Evidence.Result.RowCount != returnedRows || record.Evidence.Result.ByteCount != resultBytes {
		return DatabaseEvidenceRecord{}, fmt.Errorf("database evidence metrics differ from its actual result")
	}
	if err := validateDatabaseEvidenceRecord(record); err != nil {
		return DatabaseEvidenceRecord{}, err
	}
	return record, nil
}
