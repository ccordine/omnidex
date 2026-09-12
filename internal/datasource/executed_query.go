package datasource

import (
	"fmt"
	"strconv"
	"time"
)

// ExecutedQuery records the statement and arguments sent to PostgreSQL. It is
// evidence, never an execution input: the executor compiles a relational plan.
type ExecutedQuery struct {
	SQL        string              `json:"sql"`
	Parameters []ExecutedParameter `json:"parameters"`
}

type ExecutedParameter struct {
	Position int    `json:"position"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

func (query CompiledQuery) executionEvidence() (ExecutedQuery, error) {
	result := ExecutedQuery{SQL: query.SQL, Parameters: make([]ExecutedParameter, len(query.Parameters))}
	for index, parameter := range query.Parameters {
		var value string
		switch typed := parameter.value.(type) {
		case string:
			value = typed
		case int64:
			value = strconv.FormatInt(typed, 10)
		case bool:
			value = strconv.FormatBool(typed)
		case time.Time:
			value = typed.Format(time.RFC3339Nano)
		default:
			return ExecutedQuery{}, fmt.Errorf("compiled parameter %d has unsupported value type %T", index+1, parameter.value)
		}
		result.Parameters[index] = ExecutedParameter{Position: parameter.Position, Type: parameter.Type, Value: value}
	}
	return result, nil
}
