package datasource

const CompiledQueryV1 = "omnidex.compiled-postgres-query.v1"

type CompiledParameter struct {
	Position int    `json:"position"`
	Type     string `json:"type"`
	value    any
}

type CompiledOutput struct {
	Name         string             `json:"name"`
	FieldID      string             `json:"field_id,omitempty"`
	Aggregate    AggregateOperation `json:"aggregate,omitempty"`
	TypeCategory ColumnTypeCategory `json:"type_category"`
}

type CompiledQuery struct {
	Schema            string              `json:"schema"`
	SourceID          string              `json:"source_id"`
	SchemaFingerprint string              `json:"schema_fingerprint"`
	IntentHash        string              `json:"intent_hash"`
	QueryHash         string              `json:"query_hash"`
	SQL               string              `json:"-"`
	Parameters        []CompiledParameter `json:"parameters"`
	Outputs           []CompiledOutput    `json:"outputs"`
	Limit             int                 `json:"limit"`
	seal              [32]byte
}

func (query CompiledQuery) Arguments() []any {
	arguments := make([]any, len(query.Parameters))
	for index, parameter := range query.Parameters {
		arguments[index] = parameter.value
	}
	return arguments
}
