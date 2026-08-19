package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ExecuteEvidence(
	ctx context.Context,
	pool *pgxpool.Pool,
	snapshot SchemaSnapshot,
	compiled CompiledQuery,
	limits ExecutionLimits,
) (EvidenceResult, error) {
	if err := snapshot.ValidateIntegrity(); err != nil {
		return EvidenceResult{}, err
	}
	if err := validateCompiledQuery(snapshot, compiled); err != nil {
		return EvidenceResult{}, err
	}
	if err := validateExecutionLimits(compiled, limits); err != nil {
		return EvidenceResult{}, err
	}
	if pool == nil {
		return EvidenceResult{}, fmt.Errorf("execute database evidence requires a PostgreSQL pool")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return EvidenceResult{}, fmt.Errorf("begin read-only evidence transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := setExecutionTimeouts(ctx, tx, limits); err != nil {
		return EvidenceResult{}, err
	}
	current, err := inspectCatalog(ctx, tx, snapshot.SourceID, snapshot.SourceName, time.Now().UTC())
	if err != nil {
		return EvidenceResult{}, fmt.Errorf("reinspect schema before evidence query: %w", err)
	}
	if current.Fingerprint != snapshot.Fingerprint {
		return EvidenceResult{}, fmt.Errorf("schema fingerprint changed before evidence execution")
	}
	plan, err := explainCompiledQuery(ctx, tx, compiled)
	if err != nil {
		return EvidenceResult{}, err
	}
	if plan.TotalCost > limits.MaxTotalCost {
		return EvidenceResult{}, fmt.Errorf("query plan total cost %.2f exceeds %.2f", plan.TotalCost, limits.MaxTotalCost)
	}
	if plan.EstimatedRows > limits.MaxPlanRows {
		return EvidenceResult{}, fmt.Errorf("query plan estimates %d rows, exceeding %d", plan.EstimatedRows, limits.MaxPlanRows)
	}
	result, err := executeCompiledQuery(ctx, tx, compiled, limits)
	if err != nil {
		return EvidenceResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EvidenceResult{}, fmt.Errorf("commit evidence transaction: %w", err)
	}
	return EvidenceResult{
		Schema: EvidenceResultV1,
		Provenance: EvidenceProvenance{
			SourceID: snapshot.SourceID, SchemaFingerprint: snapshot.Fingerprint,
			IntentHash: compiled.IntentHash, QueryHash: compiled.QueryHash, ResultHash: result.Hash,
			Plan: plan, AcquiredAt: time.Now().UTC(),
		},
		Result: result,
	}, nil
}

func validateCompiledQuery(snapshot SchemaSnapshot, query CompiledQuery) error {
	if query.Schema != CompiledQueryV1 || query.SourceID != snapshot.SourceID || query.SchemaFingerprint != snapshot.Fingerprint {
		return fmt.Errorf("compiled query authority does not match schema snapshot")
	}
	if query.IntentHash == "" || query.QueryHash == "" || query.SQL == "" || query.Limit <= 0 || len(query.Outputs) == 0 {
		return fmt.Errorf("compiled query is incomplete")
	}
	queryDigest := sha256.Sum256([]byte(query.SQL))
	if hex.EncodeToString(queryDigest[:]) != query.QueryHash {
		return fmt.Errorf("compiled query SQL hash does not match")
	}
	for index, parameter := range query.Parameters {
		if parameter.Position != index+1 || parameter.Type == "" || parameter.value == nil {
			return fmt.Errorf("compiled query parameter %d is invalid", index+1)
		}
	}
	for index, output := range query.Outputs {
		if output.Name != fmt.Sprintf("c%d", index+1) || !validTypeCategory(output.TypeCategory) {
			return fmt.Errorf("compiled query output %d is invalid", index+1)
		}
	}
	if query.seal != compiledQuerySeal(query) {
		return fmt.Errorf("compiled query is not sealed by the relational compiler")
	}
	return nil
}

func validateExecutionLimits(query CompiledQuery, limits ExecutionLimits) error {
	return validateExecutionBounds(query.Limit, limits)
}

func validateExecutionBounds(limit int, limits ExecutionLimits) error {
	if math.IsNaN(limits.MaxTotalCost) || math.IsInf(limits.MaxTotalCost, 0) || limits.MaxTotalCost <= 0 || limits.MaxTotalCost > MaxEvidencePlanCost || limits.MaxPlanRows <= 0 || limits.MaxPlanRows > MaxEvidencePlanRows || limits.MaxRows <= 0 || limits.MaxRows > MaxIntentRows || limits.MaxBytes <= 0 || limits.MaxBytes > MaxEvidenceResultBytes {
		return fmt.Errorf("execution cost, plan-row, result-row, or byte limit is outside its hard bound")
	}
	if limits.StatementTimeout < time.Millisecond || limits.StatementTimeout > time.Minute {
		return fmt.Errorf("statement timeout must be within 1ms..1m")
	}
	if limits.LockTimeout < time.Millisecond || limits.LockTimeout > 10*time.Second {
		return fmt.Errorf("lock timeout must be within 1ms..10s")
	}
	if limit <= 0 || limit > limits.MaxRows {
		return fmt.Errorf("query limit %d exceeds execution row limit %d", limit, limits.MaxRows)
	}
	return nil
}

func setExecutionTimeouts(ctx context.Context, tx pgx.Tx, limits ExecutionLimits) error {
	statement := fmt.Sprintf("%dms", limits.StatementTimeout.Milliseconds())
	lock := fmt.Sprintf("%dms", limits.LockTimeout.Milliseconds())
	if _, err := tx.Exec(ctx, `SELECT pg_catalog.set_config('statement_timeout', $1, true), pg_catalog.set_config('lock_timeout', $2, true)`, statement, lock); err != nil {
		return fmt.Errorf("set transaction-local evidence timeouts: %w", err)
	}
	return nil
}

func explainCompiledQuery(ctx context.Context, tx pgx.Tx, query CompiledQuery) (ExecutionPlan, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+query.SQL, query.Arguments()...).Scan(&raw); err != nil {
		return ExecutionPlan{}, fmt.Errorf("explain evidence query: %w", err)
	}
	return decodeExecutionPlan(raw)
}

func decodeExecutionPlan(raw []byte) (ExecutionPlan, error) {
	var documents []struct {
		Plan struct {
			TotalCost float64 `json:"Total Cost"`
			PlanRows  float64 `json:"Plan Rows"`
		} `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &documents); err != nil {
		return ExecutionPlan{}, fmt.Errorf("decode PostgreSQL EXPLAIN JSON: %w", err)
	}
	if len(documents) != 1 || documents[0].Plan.TotalCost < 0 || documents[0].Plan.PlanRows < 0 {
		return ExecutionPlan{}, fmt.Errorf("PostgreSQL EXPLAIN returned an invalid top-level plan")
	}
	return ExecutionPlan{TotalCost: documents[0].Plan.TotalCost, EstimatedRows: int64(math.Ceil(documents[0].Plan.PlanRows))}, nil
}

func executeCompiledQuery(ctx context.Context, tx pgx.Tx, query CompiledQuery, limits ExecutionLimits) (TypedEvidenceResult, error) {
	rows, err := tx.Query(ctx, query.SQL, query.Arguments()...)
	if err != nil {
		return TypedEvidenceResult{}, fmt.Errorf("execute evidence query: %w", err)
	}
	defer rows.Close()
	fields := rows.FieldDescriptions()
	if len(fields) != len(query.Outputs) {
		return TypedEvidenceResult{}, fmt.Errorf("evidence query returned %d columns; expected %d", len(fields), len(query.Outputs))
	}
	columns := make([]EvidenceColumn, len(fields))
	for index, field := range fields {
		output := query.Outputs[index]
		if field.Name != output.Name {
			return TypedEvidenceResult{}, fmt.Errorf("evidence query column %d is %q; expected %q", index+1, field.Name, output.Name)
		}
		columns[index] = EvidenceColumn{Name: output.Name, PostgresTypeOID: field.DataTypeOID, FieldID: output.FieldID, Aggregate: output.Aggregate, TypeCategory: output.TypeCategory}
	}
	columnBytes, err := json.Marshal(columns)
	if err != nil {
		return TypedEvidenceResult{}, fmt.Errorf("encode evidence columns: %w", err)
	}
	byteCount := len(columnBytes)
	resultRows := [][]EvidenceValue{}
	for rows.Next() {
		if len(resultRows) >= limits.MaxRows {
			return TypedEvidenceResult{}, fmt.Errorf("evidence result exceeds %d rows", limits.MaxRows)
		}
		values, err := rows.Values()
		if err != nil {
			return TypedEvidenceResult{}, fmt.Errorf("decode evidence row: %w", err)
		}
		if len(values) != len(columns) {
			return TypedEvidenceResult{}, fmt.Errorf("evidence row has %d values; expected %d", len(values), len(columns))
		}
		row := make([]EvidenceValue, len(values))
		for index, value := range values {
			row[index], err = normalizeEvidenceValue(columns[index].PostgresTypeOID, value)
			if err != nil {
				return TypedEvidenceResult{}, fmt.Errorf("normalize evidence column %d: %w", index+1, err)
			}
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return TypedEvidenceResult{}, fmt.Errorf("encode evidence row: %w", err)
		}
		byteCount += len(encoded)
		if byteCount > limits.MaxBytes {
			return TypedEvidenceResult{}, fmt.Errorf("evidence result exceeds %d bytes", limits.MaxBytes)
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return TypedEvidenceResult{}, fmt.Errorf("iterate evidence rows: %w", err)
	}
	return finalizeTypedResult(columns, resultRows, byteCount)
}
