package datasource

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCompilePostgresBuildsParameterizedAggregateQueryAndForcedJoin(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders, customers := findRelation(t, snapshot, "orders"), findRelation(t, snapshot, "customers")
	name := findColumn(t, customers, "name")
	total := findColumn(t, orders, "total")
	created := findColumn(t, orders, "created_at")
	intent := RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID, SchemaFingerprint: snapshot.Fingerprint,
		FromRelationID: orders.ID, Shape: ResultRanking,
		Projections:     []RelationalProjection{{FieldID: name.ID}, {Aggregate: AggregateSum, FieldID: total.ID}},
		Filters:         []RelationalPredicate{{FieldID: total.ID, Operator: FilterGTE, Values: []IntentLiteral{{Type: LiteralDecimal, Value: "10.50"}}}},
		TemporalWindows: []TemporalWindow{{FieldID: created.ID, Unit: WindowDay, Amount: 90, AsOf: "2026-08-18T12:00:00Z"}},
		GroupBy:         []int{0}, Having: []AggregatePredicate{{Aggregate: AggregateSum, FieldID: total.ID, Operator: FilterGT, Value: IntentLiteral{Type: LiteralDecimal, Value: "100"}}},
		OrderBy: []OrderTerm{{Projection: 1, Direction: OrderDescending}}, Limit: 20,
	}
	compiled, err := CompilePostgres(snapshot, intent)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT t1."name" AS "c1", SUM(t0."total") AS "c2" FROM "public"."orders" AS t0 JOIN "public"."customers" AS t1 ON t0."customer_id" = t1."id" WHERE t0."total" >= $1 AND t0."created_at" >= $2 - $3::interval AND t0."created_at" <= $2 GROUP BY t1."name" HAVING SUM(t0."total") > $4 ORDER BY 2 DESC LIMIT $5`
	if compiled.SQL != want {
		t.Fatalf("SQL mismatch\n got: %s\nwant: %s", compiled.SQL, want)
	}
	arguments := compiled.Arguments()
	if len(arguments) != 5 || arguments[0] != "10.50" || arguments[2] != "90 days" || arguments[3] != "100" || arguments[4] != int64(20) {
		t.Fatalf("unexpected bound arguments: %#v", arguments)
	}
	if compiled.IntentHash == "" || compiled.QueryHash == "" || len(compiled.Outputs) != 2 {
		t.Fatalf("compiled provenance is incomplete: %#v", compiled)
	}
	for _, literal := range []string{"10.50", "2026-08-18", "90 days", "100", "LIMIT 20"} {
		if strings.Contains(compiled.SQL, literal) {
			t.Fatalf("model-authored literal %q was embedded into SQL", literal)
		}
	}
}

func TestCompilePostgresSupportsRecordsScalarTrendExistenceAndAntiExistence(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders, customers := findRelation(t, snapshot, "orders"), findRelation(t, snapshot, "customers")
	orderID := findColumn(t, orders, "id")
	status := findColumn(t, orders, "status")
	created := findColumn(t, orders, "created_at")
	customerID := findColumn(t, customers, "id")
	tests := map[string]RelationalIntent{
		"records": {
			FromRelationID: orders.ID, Shape: ResultRecords,
			Projections: []RelationalProjection{{FieldID: orderID.ID}},
			Filters:     []RelationalPredicate{{FieldID: status.ID, Operator: FilterIn, Values: []IntentLiteral{{Type: LiteralString, Value: "paid"}, {Type: LiteralString, Value: "shipped"}}}}, Limit: 8,
		},
		"scalar": {
			FromRelationID: orders.ID, Shape: ResultScalar,
			Projections: []RelationalProjection{{Aggregate: AggregateCountDistinct, FieldID: orderID.ID}}, Limit: 1,
		},
		"trend": {
			FromRelationID: orders.ID, Shape: ResultTrend,
			Projections: []RelationalProjection{{FieldID: created.ID, TimeBucket: BucketMonth}, {Aggregate: AggregateCountRows}}, GroupBy: []int{0}, OrderBy: []OrderTerm{{Projection: 0, Direction: OrderAscending}}, Limit: 24,
		},
		"existence": {
			FromRelationID: customers.ID, Shape: ResultExistence,
			Filters: []RelationalPredicate{{FieldID: customerID.ID, Operator: FilterEqual, Values: []IntentLiteral{{Type: LiteralInteger, Value: "7"}}}},
			Exists:  []ExistencePredicate{{RelationID: orders.ID, Filters: []RelationalPredicate{{FieldID: status.ID, Operator: FilterEqual, Values: []IntentLiteral{{Type: LiteralString, Value: "paid"}}}}}}, Limit: 1,
		},
		"anti existence": {
			FromRelationID: customers.ID, Shape: ResultRecords,
			Projections: []RelationalProjection{{FieldID: customerID.ID}}, Exists: []ExistencePredicate{{RelationID: orders.ID, Negated: true}}, Limit: 10,
		},
	}
	for name, intent := range tests {
		intent.Schema, intent.SourceID, intent.SchemaFingerprint = RelationalIntentV1, snapshot.SourceID, snapshot.Fingerprint
		t.Run(name, func(t *testing.T) {
			compiled, err := CompilePostgres(snapshot, intent)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(compiled.SQL, "SELECT *") || !strings.Contains(compiled.SQL, "LIMIT $") {
				t.Fatalf("unbounded or wide SQL: %s", compiled.SQL)
			}
		})
	}
}

func TestCompilePostgresRejectsAmbiguousJoinRatherThanChoosingFirst(t *testing.T) {
	t.Parallel()
	snapshot, err := NewSchemaSnapshot("ambiguous", "Ambiguous", []RelationDefinition{
		{Schema: "public", Name: "people", Kind: RelationTable, Columns: []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}, {Name: "name", Ordinal: 2, DataType: "text", TypeCategory: TypeText}}, PrimaryKey: []string{"id"}},
		{Schema: "public", Name: "messages", Kind: RelationTable, Columns: []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}, {Name: "sender_id", Ordinal: 2, DataType: "bigint", TypeCategory: TypeInteger}, {Name: "recipient_id", Ordinal: 3, DataType: "bigint", TypeCategory: TypeInteger}}, ForeignKeys: []ForeignKeyDefinition{
			{Name: "sender", Columns: []string{"sender_id"}, ReferencedSchema: "public", ReferencedRelation: "people", ReferencedColumns: []string{"id"}},
			{Name: "recipient", Columns: []string{"recipient_id"}, ReferencedSchema: "public", ReferencedRelation: "people", ReferencedColumns: []string{"id"}},
		}},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	messages, people := findRelation(t, snapshot, "messages"), findRelation(t, snapshot, "people")
	name := findColumn(t, people, "name")
	intent := RelationalIntent{Schema: RelationalIntentV1, SourceID: snapshot.SourceID, SchemaFingerprint: snapshot.Fingerprint, FromRelationID: messages.ID, Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: name.ID}}, Limit: 10}
	_, compileErr := CompilePostgres(snapshot, intent)
	if compileErr == nil {
		t.Fatal("ambiguous join was compiled")
	}
	var candidates *AmbiguousJoinPathError
	if !errors.As(compileErr, &candidates) {
		t.Fatalf("expected path candidates, got %v", compileErr)
	}
	selected := ""
	senderForeignKeyID := ""
	for _, foreignKey := range messages.ForeignKeys {
		if foreignKey.Name == "sender" {
			senderForeignKeyID = foreignKey.ID
		}
	}
	for _, candidate := range candidates.Candidates {
		if candidate.Steps[0].ForeignKeyID == senderForeignKeyID {
			selected = candidate.ID
		}
	}
	compiled, err := CompilePostgresWithJoinPaths(snapshot, intent, map[string]string{people.ID: selected})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compiled.SQL, `t0."sender_id" = t1."id"`) || strings.Contains(compiled.SQL, `recipient_id`) {
		t.Fatalf("selected path was not honored: %s", compiled.SQL)
	}
	if _, err := CompilePostgresWithJoinPaths(snapshot, intent, map[string]string{people.ID: "path_stale"}); err == nil {
		t.Fatal("stale join path candidate was accepted")
	}
}
