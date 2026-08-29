package datasource

import "time"

const (
	EvidenceResultV1       = "omnidex.database-evidence.v1"
	EvidenceProjectionV1   = "omnidex.database-evidence-projection.v1"
	MaxEvidenceResultBytes = 4 * 1024 * 1024
	MaxEvidencePlanRows    = 1_000_000_000
	MaxEvidencePlanCost    = 1_000_000_000_000
)

type ExecutionLimits struct {
	MaxTotalCost     float64
	MaxPlanRows      int64
	MaxRows          int
	MaxBytes         int
	StatementTimeout time.Duration
	LockTimeout      time.Duration
}

func DefaultExecutionLimits() ExecutionLimits {
	return ExecutionLimits{
		MaxTotalCost: 100000, MaxPlanRows: 1000000, MaxRows: 200, MaxBytes: 256 * 1024,
		StatementTimeout: 15 * time.Second, LockTimeout: 2 * time.Second,
	}
}

type ExecutionPlan struct {
	TotalCost     float64 `json:"total_cost"`
	EstimatedRows int64   `json:"estimated_rows"`
}

type EvidenceValueKind string

const (
	EvidenceNull      EvidenceValueKind = "null"
	EvidenceText      EvidenceValueKind = "text"
	EvidenceInteger   EvidenceValueKind = "integer"
	EvidenceDecimal   EvidenceValueKind = "decimal"
	EvidenceBoolean   EvidenceValueKind = "boolean"
	EvidenceTimestamp EvidenceValueKind = "timestamp"
	EvidenceDate      EvidenceValueKind = "date"
	EvidenceUUID      EvidenceValueKind = "uuid"
	EvidenceJSON      EvidenceValueKind = "json"
	EvidenceBinary    EvidenceValueKind = "binary"
)

type EvidenceValue struct {
	Kind  EvidenceValueKind `json:"kind"`
	Value string            `json:"value,omitempty"`
}

type EvidenceColumn struct {
	Name            string             `json:"name"`
	PostgresTypeOID uint32             `json:"postgres_type_oid"`
	FieldID         string             `json:"field_id,omitempty"`
	Aggregate       AggregateOperation `json:"aggregate,omitempty"`
	TypeCategory    ColumnTypeCategory `json:"type_category"`
}

type TypedEvidenceResult struct {
	Columns   []EvidenceColumn  `json:"columns"`
	Rows      [][]EvidenceValue `json:"rows"`
	RowCount  int               `json:"row_count"`
	ByteCount int               `json:"byte_count"`
	Hash      string            `json:"hash"`
}

type EvidenceProvenance struct {
	SourceID          string        `json:"source_id"`
	SchemaFingerprint string        `json:"schema_fingerprint"`
	IntentHash        string        `json:"intent_hash"`
	QueryHash         string        `json:"query_hash"`
	ResultHash        string        `json:"result_hash"`
	Plan              ExecutionPlan `json:"plan"`
	AcquiredAt        time.Time     `json:"acquired_at"`
}

type EvidenceResult struct {
	Schema     string              `json:"schema"`
	Provenance EvidenceProvenance  `json:"provenance"`
	Result     TypedEvidenceResult `json:"result"`
}

type EvidenceProjection struct {
	Schema            string                     `json:"schema"`
	SourceID          string                     `json:"source_id"`
	SchemaFingerprint string                     `json:"schema_fingerprint"`
	IntentHash        string                     `json:"intent_hash"`
	QueryHash         string                     `json:"query_hash"`
	ResultHash        string                     `json:"result_hash"`
	Columns           []EvidenceProjectionColumn `json:"columns"`
	Rows              [][]EvidenceValue          `json:"rows"`
	RowCount          int                        `json:"row_count"`
}

type EvidenceProjectionColumn struct {
	Name         string             `json:"name"`
	FieldID      string             `json:"field_id,omitempty"`
	Aggregate    AggregateOperation `json:"aggregate,omitempty"`
	TypeCategory ColumnTypeCategory `json:"type_category"`
}

func (evidence EvidenceResult) Projection() EvidenceProjection {
	columns := make([]EvidenceProjectionColumn, len(evidence.Result.Columns))
	for index, column := range evidence.Result.Columns {
		columns[index] = EvidenceProjectionColumn{Name: column.Name, FieldID: column.FieldID, Aggregate: column.Aggregate, TypeCategory: column.TypeCategory}
	}
	return EvidenceProjection{
		Schema: EvidenceProjectionV1, SourceID: evidence.Provenance.SourceID,
		SchemaFingerprint: evidence.Provenance.SchemaFingerprint, IntentHash: evidence.Provenance.IntentHash,
		QueryHash: evidence.Provenance.QueryHash, ResultHash: evidence.Provenance.ResultHash,
		Columns: columns, Rows: cloneEvidenceRows(evidence.Result.Rows),
		RowCount: evidence.Result.RowCount,
	}
}

func cloneEvidenceRows(rows [][]EvidenceValue) [][]EvidenceValue {
	cloned := make([][]EvidenceValue, len(rows))
	for index := range rows {
		cloned[index] = append([]EvidenceValue(nil), rows[index]...)
	}
	return cloned
}
