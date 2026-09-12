package datasource

import "fmt"

type EvidenceRowColumn struct {
	Label string             `json:"label"`
	Kind  ColumnTypeCategory `json:"kind"`
}

type EvidenceRows struct {
	Columns []EvidenceRowColumn `json:"columns"`
	Rows    [][]EvidenceValue   `json:"rows"`
}

// ProjectEvidenceRows is the single projection used both to present query
// results and to verify a citation against its stored execution.
func ProjectEvidenceRows(snapshot SchemaSnapshot, intent RelationalIntent, result TypedEvidenceResult, start, end int) (EvidenceRows, error) {
	if start < 0 || end < start || end > len(result.Rows) || result.RowCount != len(result.Rows) ||
		(start == end && (start != 0 || result.RowCount != 0)) {
		return EvidenceRows{}, fmt.Errorf("database evidence row range is outside the actual result")
	}
	projection := EvidenceRows{Rows: make([][]EvidenceValue, end-start)}
	if intent.Shape == ResultExistence {
		if len(intent.Projections) != 0 || len(result.Columns) != 1 || result.Columns[0].TypeCategory != TypeBoolean {
			return EvidenceRows{}, fmt.Errorf("database existence evidence requires one boolean column")
		}
		projection.Columns = []EvidenceRowColumn{{Label: "exists", Kind: TypeBoolean}}
	} else {
		if len(result.Columns) != len(intent.Projections) || len(result.Columns) == 0 {
			return EvidenceRows{}, fmt.Errorf("database evidence columns differ from the relational intent")
		}
		projection.Columns = make([]EvidenceRowColumn, len(result.Columns))
		for index, field := range intent.Projections {
			column := result.Columns[index]
			if column.FieldID != field.FieldID || column.Aggregate != field.Aggregate {
				return EvidenceRows{}, fmt.Errorf("database evidence column %d differs from the relational intent", index+1)
			}
			label, err := evidenceColumnLabel(snapshot, field)
			if err != nil {
				return EvidenceRows{}, err
			}
			projection.Columns[index] = EvidenceRowColumn{Label: label, Kind: column.TypeCategory}
		}
	}
	for index, row := range result.Rows[start:end] {
		if len(row) != len(projection.Columns) {
			return EvidenceRows{}, fmt.Errorf("database evidence row has the wrong column count")
		}
		projection.Rows[index] = append([]EvidenceValue(nil), row...)
	}
	return projection, nil
}

func evidenceColumnLabel(snapshot SchemaSnapshot, projection RelationalProjection) (string, error) {
	if projection.Aggregate == AggregateCountRows {
		return "count_rows", nil
	}
	relation, column, err := snapshot.Column(projection.FieldID)
	if err != nil {
		return "", err
	}
	name := relation.Schema + "." + relation.Name + "." + column.Name
	if projection.Aggregate != "" {
		return string(projection.Aggregate) + "(" + name + ")", nil
	}
	if projection.TimeBucket != "" {
		return string(projection.TimeBucket) + "(" + name + ")", nil
	}
	return name, nil
}
