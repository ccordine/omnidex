package datasource

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSchemaAndQueryEvidenceUseActualValues(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, dataType, parameter, returned string
		category                            ColumnTypeCategory
		literal                             LiteralType
		kind                                EvidenceValueKind
	}{
		{"schedule", "timestamp with time zone", "2026-09-08T12:30:00-04:00", "2026-09-08T16:30:00Z", TypeTemporal, LiteralTimestamp, EvidenceTimestamp},
		{"switches", "boolean", "false", "false", TypeBoolean, LiteralBoolean, EvidenceBoolean},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			captured := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
			definitions := []RelationDefinition{{Schema: "public", Name: fixture.name, Kind: RelationTable,
				Columns: []ColumnDefinition{{Name: "value", Ordinal: 1, DataType: fixture.dataType, TypeCategory: fixture.category}}}}
			snapshot, err := NewSchemaSnapshot("source-1", "Fixture", definitions, captured)
			if err != nil {
				t.Fatal(err)
			}
			if err := snapshot.ValidateIntegrity(); err != nil {
				t.Fatal(err)
			}
			relation := snapshot.Relations[0]
			if relation.ID != "rel_1" || relation.Columns[0].ID != "rel_1_col_1" {
				t.Fatalf("references are not snapshot-local ordinals: %#v", relation)
			}
			projection, err := ProjectSchemaForIntent(snapshot, []string{relation.ID})
			if err != nil {
				t.Fatal(err)
			}
			intent := RelationalIntent{Schema: RelationalIntentV1, SourceID: snapshot.SourceID,
				FromRelationID: relation.ID, Shape: ResultRecords, Limit: 1,
				Projections: []RelationalProjection{{FieldID: relation.Columns[0].ID}},
				Filters: []RelationalPredicate{{FieldID: relation.Columns[0].ID, Operator: FilterEqual,
					Values: []IntentLiteral{{Type: fixture.literal, Value: fixture.parameter}}}},
			}
			plan, err := BuildRelationalQueryPlan(snapshot, intent, nil)
			if err != nil {
				t.Fatal(err)
			}
			compiled, err := CompilePostgresPlan(snapshot, plan)
			if err != nil {
				t.Fatal(err)
			}
			query, err := compiled.executionEvidence()
			if err != nil || query.Parameters[0].Value != fixture.parameter {
				t.Fatalf("actual argument was not retained: %#v / %v", query, err)
			}
			output := compiled.Outputs[0]
			result := EvidenceResult{Schema: EvidenceResultV1,
				Execution: EvidenceExecution{SourceID: snapshot.SourceID, Query: query, AcquiredAt: captured, DurationMS: 12},
				Result: TypedEvidenceResult{
					Columns: []EvidenceColumn{{Name: output.Name, FieldID: output.FieldID, TypeCategory: output.TypeCategory}},
					Rows:    [][]EvidenceValue{{{Kind: fixture.kind, Value: fixture.returned}}}, RowCount: 1,
				},
			}
			setFixtureResultBytes(t, &result.Result)
			if err := result.ValidateForPlan(snapshot, plan, DefaultExecutionLimits()); err != nil {
				t.Fatal(err)
			}
			for _, value := range []any{snapshot, projection, intent, plan, compiled, result} {
				encoded, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				for _, forbidden := range []string{"fingerprint", "hash", "seal", "provenance"} {
					if strings.Contains(string(encoded), forbidden) {
						t.Fatalf("%T retained %q metadata: %s", value, forbidden, encoded)
					}
				}
			}
			for _, mutation := range []func(*EvidenceResult){
				func(e *EvidenceResult) { e.Execution.Query.SQL += " changed" },
				func(e *EvidenceResult) { e.Execution.Query.Parameters[0].Value = "changed" },
				func(e *EvidenceResult) { e.Result.ByteCount++ },
				func(e *EvidenceResult) { e.Result.Columns[0].Name = "changed" },
				func(e *EvidenceResult) { e.Result.Rows[0][0].Kind = EvidenceBinary },
				func(e *EvidenceResult) {
					e.Result.Rows = append(e.Result.Rows, e.Result.Rows[0])
					e.Result.RowCount++
					setFixtureResultBytes(t, &e.Result)
				},
			} {
				encoded, _ := json.Marshal(result)
				var changed EvidenceResult
				if err := json.Unmarshal(encoded, &changed); err != nil {
					t.Fatal(err)
				}
				mutation(&changed)
				if err := changed.ValidateForPlan(snapshot, plan, DefaultExecutionLimits()); err == nil {
					t.Fatalf("changed query, result shape, or row limit was accepted: %#v", changed)
				}
			}
			rows, err := ProjectEvidenceRows(snapshot, intent, result.Result, 0, 1)
			if err != nil || rows.Rows[0][0].Value != fixture.returned {
				t.Fatalf("row projection lost actual value: %#v / %v", rows, err)
			}
			rows.Rows[0][0].Value = "changed"
			if result.Result.Rows[0][0].Value != fixture.returned {
				t.Fatal("projection overwrote the retained result")
			}
			if _, err := ProjectEvidenceRows(snapshot, intent, result.Result, 0, 0); err == nil {
				t.Fatal("nonempty result manufactured an empty citation")
			}
			definitions[0].RowEstimate = 99
			later, err := NewSchemaSnapshot(snapshot.SourceID, snapshot.SourceName, definitions, captured.Add(time.Hour))
			if err != nil || later.Relations[0].RowEstimate != 99 ||
				later.Relations[0].ID != relation.ID || !reflect.DeepEqual(later.Relations[0].Columns, relation.Columns) {
				t.Fatalf("a later observation changed the retained column references: %v", err)
			}
			definitions[0].Columns[0].Name = "renamed"
			changed, err := NewSchemaSnapshot(snapshot.SourceID, snapshot.SourceName, definitions, captured)
			if err != nil || changed.Relations[0].Columns[0].Name != "renamed" || reflect.DeepEqual(snapshot, changed) {
				t.Fatalf("actual schema change was missed: %v", err)
			}
		})
	}
}

func setFixtureResultBytes(t *testing.T, result *TypedEvidenceResult) {
	t.Helper()
	encoded, err := json.Marshal(result.Columns)
	if err != nil {
		t.Fatal(err)
	}
	result.ByteCount = len(encoded)
	for _, row := range result.Rows {
		encoded, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		result.ByteCount += len(encoded)
	}
}
