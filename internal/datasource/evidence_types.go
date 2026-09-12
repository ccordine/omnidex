package datasource

import "time"

const (
	EvidenceResultV1       = "omnidex.database-evidence.v1"
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
}

type EvidenceExecution struct {
	SourceID   string        `json:"source_id"`
	Query      ExecutedQuery `json:"query"`
	Plan       ExecutionPlan `json:"plan"`
	AcquiredAt time.Time     `json:"acquired_at"`
	DurationMS int64         `json:"duration_ms"`
}

type EvidenceResult struct {
	Schema    string              `json:"schema"`
	Execution EvidenceExecution   `json:"execution"`
	Result    TypedEvidenceResult `json:"result"`
}
