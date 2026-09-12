package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
)

type DatabaseEvidenceRecord struct {
	ID       int64
	JobID    int64
	Snapshot datasource.SchemaSnapshot
	Plan     datasource.RelationalQueryPlan
	Evidence datasource.EvidenceResult
}

func (record DatabaseEvidenceRecord) SourceRef() string {
	return fmt.Sprintf("database:%s/query/%d", record.Evidence.Execution.SourceID, record.ID)
}

// RecordDatabaseEvidence records one completed read, not a content-addressed
// receipt. Separate executions remain separate observations even for equal rows.
func (r *Repository) RecordDatabaseEvidence(
	ctx context.Context,
	jobID int64,
	snapshot datasource.SchemaSnapshot,
	plan datasource.RelationalQueryPlan,
	result datasource.EvidenceResult,
) (DatabaseEvidenceRecord, error) {
	if ctx == nil || r == nil || r.pool == nil || jobID < 1 {
		return DatabaseEvidenceRecord{}, fmt.Errorf("record database evidence requires PostgreSQL, context, and a positive job ID")
	}
	record := DatabaseEvidenceRecord{JobID: jobID, Snapshot: snapshot, Plan: plan, Evidence: result}
	if err := validateDatabaseEvidenceRecord(record); err != nil {
		return DatabaseEvidenceRecord{}, err
	}
	schemaJSON, err := json.Marshal(snapshot)
	if err != nil {
		return DatabaseEvidenceRecord{}, fmt.Errorf("encode observed database schema: %w", err)
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return DatabaseEvidenceRecord{}, fmt.Errorf("encode executed database plan: %w", err)
	}
	parametersJSON, err := json.Marshal(result.Execution.Query.Parameters)
	if err != nil {
		return DatabaseEvidenceRecord{}, fmt.Errorf("encode executed database parameters: %w", err)
	}
	resultJSON, err := json.Marshal(result.Result)
	if err != nil {
		return DatabaseEvidenceRecord{}, fmt.Errorf("encode returned database rows: %w", err)
	}
	execution := result.Execution
	err = r.pool.QueryRow(ctx, `
		INSERT INTO database_evidence (
			job_id, data_source_id, schema_snapshot, query_plan, query_text,
			query_parameters, result_json, plan_total_cost, plan_estimated_rows,
			returned_rows, result_bytes, acquired_at, duration_ms
		)
		SELECT job.id, source.id, $3::jsonb, $4::jsonb, $5, $6::jsonb,
		       $7::jsonb, $8, $9, $10, $11, $12, $13
		FROM jobs AS job
		JOIN data_sources AS source ON source.id=job.metadata->>'data_source_id'
		JOIN ai_channels AS channel ON channel.id=job.metadata->>'channel_id'
		  AND channel.data_source_id=source.id
		WHERE job.id=$1 AND job.pipeline='chat' AND source.id=$2
		RETURNING id, acquired_at
	`, jobID, execution.SourceID, schemaJSON, planJSON, execution.Query.SQL,
		parametersJSON, resultJSON, execution.Plan.TotalCost, execution.Plan.EstimatedRows,
		result.Result.RowCount, result.Result.ByteCount, execution.AcquiredAt,
		execution.DurationMS).Scan(&record.ID, &record.Evidence.Execution.AcquiredAt)
	if err != nil {
		return DatabaseEvidenceRecord{}, fmt.Errorf("record database execution for its bound job: %w", err)
	}
	record.Evidence.Execution.AcquiredAt = record.Evidence.Execution.AcquiredAt.UTC()
	return record, nil
}

func validateDatabaseEvidenceRecord(record DatabaseEvidenceRecord) error {
	if record.JobID < 1 || model.DataSourceID(record.Evidence.Execution.SourceID).Validate() != nil {
		return fmt.Errorf("database evidence requires its owning job and source")
	}
	limits := datasource.DefaultExecutionLimits()
	limits.MaxTotalCost = datasource.MaxEvidencePlanCost
	limits.MaxPlanRows = datasource.MaxEvidencePlanRows
	limits.MaxRows = datasource.MaxIntentRows
	limits.MaxBytes = datasource.MaxEvidenceResultBytes
	return record.Evidence.ValidateForPlan(record.Snapshot, record.Plan, limits)
}
