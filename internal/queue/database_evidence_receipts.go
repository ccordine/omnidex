package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type DatabaseEvidenceReceipt struct {
	ID                int64
	JobID             int64
	DataSourceID      string
	SchemaFingerprint string
	IntentHash        string
	QueryHash         string
	ResultHash        string
	PlanTotalCost     float64
	PlanEstimatedRows int64
	ReturnedRows      int
	ResultBytes       int
	AcquiredAt        time.Time
}

func (r *Repository) SaveDatabaseEvidenceReceipt(
	ctx context.Context,
	jobID int64,
	evidence datasource.EvidenceResult,
) (DatabaseEvidenceReceipt, error) {
	if ctx == nil || r == nil || r.pool == nil || jobID < 1 {
		return DatabaseEvidenceReceipt{}, fmt.Errorf("database evidence receipt requires PostgreSQL, context, and a positive job ID")
	}
	if err := validateExactDatabaseEvidenceResult(evidence); err != nil {
		return DatabaseEvidenceReceipt{}, err
	}
	receipt := DatabaseEvidenceReceipt{
		JobID: jobID, DataSourceID: evidence.Provenance.SourceID,
		SchemaFingerprint: evidence.Provenance.SchemaFingerprint,
		IntentHash:        evidence.Provenance.IntentHash, QueryHash: evidence.Provenance.QueryHash,
		ResultHash: evidence.Provenance.ResultHash, PlanTotalCost: evidence.Provenance.Plan.TotalCost,
		PlanEstimatedRows: evidence.Provenance.Plan.EstimatedRows,
		ReturnedRows:      evidence.Result.RowCount, ResultBytes: evidence.Result.ByteCount,
		AcquiredAt: evidence.Provenance.AcquiredAt.UTC(),
	}
	if err := receipt.validate(); err != nil {
		return DatabaseEvidenceReceipt{}, err
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO database_evidence_receipts (
			job_id, data_source_id, schema_fingerprint, intent_hash, query_hash,
			result_hash, plan_total_cost, plan_estimated_rows, returned_rows,
			result_bytes, acquired_at
		)
		SELECT job.id, source.id, $3, $4, $5, $6, $7, $8, $9, $10, $11
		FROM jobs AS job
		JOIN data_sources AS source
		  ON source.id=job.metadata->>'data_source_id'
		JOIN ai_channels AS channel
		  ON channel.id=job.metadata->>'channel_id'
		 AND channel.data_source_id=source.id
		WHERE job.id=$1 AND job.pipeline='chat' AND source.id=$2
		  AND source.schema_catalog->>'fingerprint'=$3
		ON CONFLICT (job_id, schema_fingerprint, intent_hash, query_hash, result_hash) DO NOTHING
		RETURNING id
	`, jobID, receipt.DataSourceID, receipt.SchemaFingerprint, receipt.IntentHash,
		receipt.QueryHash, receipt.ResultHash, receipt.PlanTotalCost,
		receipt.PlanEstimatedRows, receipt.ReturnedRows, receipt.ResultBytes,
		receipt.AcquiredAt).Scan(&receipt.ID)
	if err == pgx.ErrNoRows {
		err = r.pool.QueryRow(ctx, `
			SELECT receipt.id
			FROM database_evidence_receipts AS receipt
			JOIN jobs AS job ON job.id=receipt.job_id
			JOIN ai_channels AS channel
			  ON channel.id=job.metadata->>'channel_id'
			 AND channel.data_source_id=receipt.data_source_id
			WHERE receipt.job_id=$1 AND receipt.data_source_id=$2
			  AND job.pipeline='chat'
			  AND job.metadata->>'data_source_id'=receipt.data_source_id
			  AND receipt.schema_fingerprint=$3 AND receipt.intent_hash=$4
			  AND receipt.query_hash=$5 AND receipt.result_hash=$6
		`, jobID, receipt.DataSourceID, receipt.SchemaFingerprint, receipt.IntentHash,
			receipt.QueryHash, receipt.ResultHash).Scan(&receipt.ID)
	}
	if err != nil {
		return DatabaseEvidenceReceipt{}, fmt.Errorf("persist bound database evidence receipt: %w", err)
	}
	return receipt, nil
}

func validateExactDatabaseEvidenceResult(evidence datasource.EvidenceResult) error {
	if evidence.Schema != datasource.EvidenceResultV1 ||
		evidence.Result.Hash != evidence.Provenance.ResultHash ||
		evidence.Result.RowCount != len(evidence.Result.Rows) ||
		len(evidence.Result.Columns) == 0 {
		return fmt.Errorf("database evidence receipt requires one exact typed execution result")
	}
	canonical, err := json.Marshal(struct {
		Columns []datasource.EvidenceColumn  `json:"columns"`
		Rows    [][]datasource.EvidenceValue `json:"rows"`
	}{Columns: evidence.Result.Columns, Rows: evidence.Result.Rows})
	if err != nil {
		return fmt.Errorf("encode database evidence receipt result: %w", err)
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != evidence.Result.Hash {
		return fmt.Errorf("database evidence receipt result hash does not match its typed rows")
	}
	columns, err := json.Marshal(evidence.Result.Columns)
	if err != nil {
		return fmt.Errorf("encode database evidence receipt columns: %w", err)
	}
	byteCount := len(columns)
	for _, row := range evidence.Result.Rows {
		encoded, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("encode database evidence receipt row: %w", err)
		}
		byteCount += len(encoded)
	}
	if evidence.Result.ByteCount != byteCount {
		return fmt.Errorf("database evidence receipt byte count does not match its typed result")
	}
	return nil
}

func (receipt DatabaseEvidenceReceipt) validate() error {
	if receipt.JobID < 1 || model.DataSourceID(receipt.DataSourceID).Validate() != nil ||
		math.IsNaN(receipt.PlanTotalCost) || math.IsInf(receipt.PlanTotalCost, 0) ||
		receipt.PlanTotalCost < 0 || receipt.PlanTotalCost > datasource.MaxEvidencePlanCost ||
		receipt.PlanEstimatedRows < 0 || receipt.PlanEstimatedRows > datasource.MaxEvidencePlanRows ||
		receipt.ReturnedRows < 0 || receipt.ReturnedRows > datasource.MaxIntentRows ||
		receipt.ResultBytes < 0 || receipt.ResultBytes > datasource.MaxEvidenceResultBytes ||
		receipt.AcquiredAt.IsZero() {
		return fmt.Errorf("database evidence receipt contains invalid identity or metrics")
	}
	for label, value := range map[string]string{
		"schema fingerprint": receipt.SchemaFingerprint,
		"intent hash":        receipt.IntentHash,
		"query hash":         receipt.QueryHash,
		"result hash":        receipt.ResultHash,
	} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || value != strings.ToLower(value) {
			return fmt.Errorf("database evidence receipt %s is not an exact SHA-256", label)
		}
	}
	return nil
}
