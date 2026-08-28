package datasource

const (
	RelationalIntentV1           = "omnidex.relational-intent.v1"
	MaxIntentBytes               = 16 * 1024
	MaxIntentProjections         = 16
	MaxIntentFilters             = 24
	MaxIntentFilterValues        = 50
	MaxIntentGroups              = 8
	MaxIntentOrderTerms          = 8
	MaxIntentExistenceChecks     = 8
	MaxIntentRows                = 500
	MaxIntentStringLiteralBytes  = 2 * 1024
	MaxIntentDecimalLiteralBytes = 128
)

type ResultShape string

const (
	ResultRecords      ResultShape = "records"
	ResultScalar       ResultShape = "scalar"
	ResultRanking      ResultShape = "ranking"
	ResultDistribution ResultShape = "distribution"
	ResultComparison   ResultShape = "comparison"
	ResultTrend        ResultShape = "trend"
	ResultExistence    ResultShape = "existence"
)

type AggregateOperation string

const (
	AggregateCountRows     AggregateOperation = "count_rows"
	AggregateCount         AggregateOperation = "count"
	AggregateCountDistinct AggregateOperation = "count_distinct"
	AggregateSum           AggregateOperation = "sum"
	AggregateAverage       AggregateOperation = "average"
	AggregateMinimum       AggregateOperation = "minimum"
	AggregateMaximum       AggregateOperation = "maximum"
)

type TimeBucketUnit string

const (
	BucketDay     TimeBucketUnit = "day"
	BucketWeek    TimeBucketUnit = "week"
	BucketMonth   TimeBucketUnit = "month"
	BucketQuarter TimeBucketUnit = "quarter"
	BucketYear    TimeBucketUnit = "year"
)

type FilterOperator string

const (
	FilterEqual     FilterOperator = "eq"
	FilterNotEqual  FilterOperator = "neq"
	FilterGT        FilterOperator = "gt"
	FilterGTE       FilterOperator = "gte"
	FilterLT        FilterOperator = "lt"
	FilterLTE       FilterOperator = "lte"
	FilterIn        FilterOperator = "in"
	FilterNotIn     FilterOperator = "not_in"
	FilterIsNull    FilterOperator = "is_null"
	FilterIsNotNull FilterOperator = "is_not_null"
	FilterContains  FilterOperator = "contains"
	FilterPrefix    FilterOperator = "prefix"
)

type LiteralType string

const (
	LiteralString    LiteralType = "string"
	LiteralInteger   LiteralType = "integer"
	LiteralDecimal   LiteralType = "decimal"
	LiteralBoolean   LiteralType = "boolean"
	LiteralTimestamp LiteralType = "timestamp"
	LiteralDate      LiteralType = "date"
	LiteralUUID      LiteralType = "uuid"
)

type WindowUnit string

const (
	WindowHour  WindowUnit = "hour"
	WindowDay   WindowUnit = "day"
	WindowWeek  WindowUnit = "week"
	WindowMonth WindowUnit = "month"
	WindowYear  WindowUnit = "year"
)

type OrderDirection string

const (
	OrderAscending  OrderDirection = "asc"
	OrderDescending OrderDirection = "desc"
)

type IntentLiteral struct {
	Type  LiteralType `json:"type"`
	Value string      `json:"value"`
}

type RelationalProjection struct {
	FieldID    string             `json:"field_id"`
	Aggregate  AggregateOperation `json:"aggregate"`
	TimeBucket TimeBucketUnit     `json:"time_bucket"`
}

type RelationalPredicate struct {
	FieldID  string          `json:"field_id"`
	Operator FilterOperator  `json:"operator"`
	Values   []IntentLiteral `json:"values"`
}

type TemporalWindow struct {
	FieldID string     `json:"field_id"`
	Unit    WindowUnit `json:"unit"`
	Amount  int        `json:"amount"`
	AsOf    string     `json:"as_of"`
}

type AggregatePredicate struct {
	Aggregate AggregateOperation `json:"aggregate"`
	FieldID   string             `json:"field_id"`
	Operator  FilterOperator     `json:"operator"`
	Value     IntentLiteral      `json:"value"`
}

type OrderTerm struct {
	Projection int            `json:"projection"`
	Direction  OrderDirection `json:"direction"`
}

type ExistencePredicate struct {
	RelationID string                `json:"relation_id"`
	Negated    bool                  `json:"negated"`
	Filters    []RelationalPredicate `json:"filters"`
}

type RelationalIntent struct {
	Schema            string                 `json:"schema"`
	SourceID          string                 `json:"source_id"`
	SchemaFingerprint string                 `json:"schema_fingerprint"`
	FromRelationID    string                 `json:"from_relation_id"`
	Shape             ResultShape            `json:"shape"`
	Projections       []RelationalProjection `json:"projections"`
	Filters           []RelationalPredicate  `json:"filters"`
	TemporalWindows   []TemporalWindow       `json:"temporal_windows"`
	Exists            []ExistencePredicate   `json:"exists"`
	GroupBy           []int                  `json:"group_by"`
	Having            []AggregatePredicate   `json:"having"`
	OrderBy           []OrderTerm            `json:"order_by"`
	Limit             int                    `json:"limit"`
}
