package datasource

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/omni"
)

const MaxQueryRows = 500

type QueryResult struct {
	SQL     string           `json:"sql,omitempty"`
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Count   int              `json:"count"`
}

func enforceQueryLimit(sql string, max int) string {
	if max <= 0 || strings.Contains(strings.ToLower(sql), " limit ") {
		return sql
	}
	return strings.TrimRight(strings.TrimSpace(sql), ";") + fmt.Sprintf(" LIMIT %d", max)
}

func rowsToColumns(rows []omni.MemorySQLRow) ([]string, []map[string]any) {
	columns := []string{}
	seen := map[string]struct{}{}
	for _, row := range rows {
		for key := range row {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			columns = append(columns, key)
		}
	}
	publicRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out := map[string]any{}
		for key, value := range row {
			out[key] = stringifyCell(value)
		}
		publicRows = append(publicRows, out)
	}
	return columns, publicRows
}

func stringifyCell(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339)
	default:
		return typed
	}
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}
