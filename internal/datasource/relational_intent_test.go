package datasource

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func commerceSnapshot(t *testing.T) SchemaSnapshot {
	t.Helper()
	snapshot, err := NewSchemaSnapshot("commerce-source", "Commerce", []RelationDefinition{
		{
			Schema: "public", Name: "customers", Kind: RelationTable,
			Columns: []ColumnDefinition{
				{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger},
				{Name: "name", Ordinal: 2, DataType: "text", TypeCategory: TypeText},
			},
			PrimaryKey: []string{"id"},
		},
		{
			Schema: "public", Name: "orders", Kind: RelationTable,
			Columns: []ColumnDefinition{
				{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger},
				{Name: "customer_id", Ordinal: 2, DataType: "bigint", TypeCategory: TypeInteger},
				{Name: "status", Ordinal: 3, DataType: "order_status", TypeCategory: TypeText, AllowedValues: []string{"open", "paid", "shipped"}},
				{Name: "total", Ordinal: 4, DataType: "numeric", TypeCategory: TypeDecimal},
				{Name: "created_at", Ordinal: 5, DataType: "timestamp with time zone", TypeCategory: TypeTemporal},
			},
			PrimaryKey:  []string{"id"},
			ForeignKeys: []ForeignKeyDefinition{{Name: "orders_customer_fk", Columns: []string{"customer_id"}, ReferencedSchema: "public", ReferencedRelation: "customers", ReferencedColumns: []string{"id"}}},
		},
	}, time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func findRelation(t *testing.T, snapshot SchemaSnapshot, name string) SchemaRelation {
	t.Helper()
	for _, relation := range snapshot.Relations {
		if relation.Name == name {
			return relation
		}
	}
	t.Fatalf("relation %q not found", name)
	return SchemaRelation{}
}

func findColumn(t *testing.T, relation SchemaRelation, name string) SchemaColumn {
	t.Helper()
	for _, column := range relation.Columns {
		if column.Name == name {
			return column
		}
	}
	t.Fatalf("column %q not found", name)
	return SchemaColumn{}
}

func TestRelationalIntentEnforcesShapeAndTypeSemantics(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders := findRelation(t, snapshot, "orders")
	status := findColumn(t, orders, "status")
	created := findColumn(t, orders, "created_at")
	cases := map[string]RelationalIntent{
		"record aggregate": {
			Shape: ResultRecords, Projections: []RelationalProjection{{Aggregate: AggregateCountRows}}, Limit: 5,
		},
		"ranking without group": {
			Shape: ResultRanking, Projections: []RelationalProjection{{Aggregate: AggregateCountRows}}, OrderBy: []OrderTerm{{Projection: 0, Direction: OrderDescending}}, Limit: 5,
		},
		"text numeric comparison": {
			Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: status.ID}}, Filters: []RelationalPredicate{{FieldID: status.ID, Operator: FilterGT, Values: []IntentLiteral{{Type: LiteralInteger, Value: "4"}}}}, Limit: 5,
		},
		"temporal wrong field": {
			Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: status.ID}}, TemporalWindows: []TemporalWindow{{FieldID: status.ID, Unit: WindowDay, Amount: 3, AsOf: "2026-08-18T00:00:00Z"}}, Limit: 5,
		},
		"unknown enum value": {
			Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: status.ID}}, Filters: []RelationalPredicate{{FieldID: status.ID, Operator: FilterEqual, Values: []IntentLiteral{{Type: LiteralString, Value: "cancelled"}}}}, Limit: 5,
		},
		"enum text pattern": {
			Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: status.ID}}, Filters: []RelationalPredicate{{FieldID: status.ID, Operator: FilterContains, Values: []IntentLiteral{{Type: LiteralString, Value: "paid"}}}}, Limit: 5,
		},
		"trend without bucket": {
			Shape: ResultTrend, Projections: []RelationalProjection{{FieldID: created.ID}, {Aggregate: AggregateCountRows}}, GroupBy: []int{0}, Limit: 5,
		},
	}
	for name, intent := range cases {
		intent.Schema = RelationalIntentV1
		intent.SourceID = snapshot.SourceID
		intent.SchemaFingerprint = snapshot.Fingerprint
		intent.FromRelationID = orders.ID
		if err := intent.Validate(snapshot); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestProjectSchemaForIntentContainsNoConnectionPromptOrOperationAuthority(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	projection, err := ProjectSchemaForIntent(snapshot, []string{snapshot.Relations[0].ID, snapshot.Relations[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	visible := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"password", "postgres://", "context_prompt", "domainguidance", "select *", "run_sql", "tool_call", "execute"} {
		if strings.Contains(visible, forbidden) {
			t.Errorf("model-visible schema projection contains forbidden %q: %s", forbidden, visible)
		}
	}
}
