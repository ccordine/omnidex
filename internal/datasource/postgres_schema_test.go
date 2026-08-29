package datasource

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresTypeCategoryIsMechanical(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		code, postgresType string
		want               ColumnTypeCategory
	}{
		"bigint":      {"N", "bigint", TypeInteger},
		"numeric":     {"N", "numeric(12,2)", TypeDecimal},
		"varchar":     {"S", "character varying(80)", TypeText},
		"boolean":     {"B", "boolean", TypeBoolean},
		"timestamp":   {"D", "timestamp with time zone", TypeTemporal},
		"date":        {"D", "date", TypeDate},
		"uuid":        {"U", "uuid", TypeUUID},
		"jsonb":       {"U", "jsonb", TypeJSON},
		"bytea":       {"U", "bytea", TypeBinary},
		"enum":        {"E", "order_status", TypeText},
		"interval":    {"T", "interval", TypeOther},
		"unsupported": {"U", "inet", TypeOther},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := postgresTypeCategory(test.code, test.postgresType); got != test.want {
				t.Fatalf("category=%q want=%q", got, test.want)
			}
		})
	}
}

func TestInspectCatalogCapturesPostgresAuthority(t *testing.T) {
	dsn := os.Getenv("OMNI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := fmt.Sprintf("omnidex_datasource_%d", time.Now().UnixNano())
	quotedSchema := quoteIdentifier(schema)
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE") }()
	statements := []string{
		"CREATE TYPE " + quotedSchema + `.order_status AS ENUM ('open', 'paid', 'shipped')`,
		"CREATE TABLE " + quotedSchema + `.customers (id bigint PRIMARY KEY, email text NOT NULL UNIQUE)`,
		"CREATE TABLE " + quotedSchema + `.orders (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, customer_id bigint NOT NULL, status ` + quotedSchema + `.order_status NOT NULL, amount numeric(12,2) NOT NULL, created_at timestamptz NOT NULL, reference uuid NOT NULL, active boolean NOT NULL, note text, CONSTRAINT orders_customer_fk FOREIGN KEY (customer_id) REFERENCES ` + quotedSchema + `.customers(id), CONSTRAINT orders_status_check CHECK (status::text <> ''))`,
		"CREATE INDEX orders_created_idx ON " + quotedSchema + `.orders (created_at) WHERE status = 'paid'`,
		"CREATE VIEW " + quotedSchema + `.paid_orders AS SELECT id, customer_id, amount FROM ` + quotedSchema + `.orders WHERE status = 'paid'`,
		"INSERT INTO " + quotedSchema + `.customers (id, email) VALUES (1, 'one@example.test'), (2, 'two@example.test')`,
		"INSERT INTO " + quotedSchema + `.orders (customer_id, status, amount, created_at, reference, active, note) VALUES (1, 'paid', 10.50, '2026-08-18T12:00:00Z', '123e4567-e89b-12d3-a456-426614174000', true, NULL), (2, 'open', 4.25, '2026-08-17T12:00:00Z', '123e4567-e89b-12d3-a456-426614174001', false, 'second')`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	one, err := InspectCatalog(ctx, pool, "integration-source", "Integration")
	if err != nil {
		t.Fatal(err)
	}
	two, err := InspectCatalog(ctx, pool, "integration-source", "Different display name")
	if err != nil {
		t.Fatal(err)
	}
	if one.Fingerprint != two.Fingerprint {
		t.Fatalf("stable catalog changed fingerprint: %s != %s", one.Fingerprint, two.Fingerprint)
	}
	orders := relationByQualifiedName(t, one, schema, "orders")
	view := relationByQualifiedName(t, one, schema, "paid_orders")
	if view.Kind != RelationView {
		t.Fatalf("view kind=%q", view.Kind)
	}
	if orders.PrimaryKeyID == "" || orders.PrimaryKeyName == "" || len(orders.PrimaryKey) != 1 || len(orders.ForeignKeys) != 1 || len(orders.CheckConstraints) != 1 {
		t.Fatalf("constraint metadata incomplete: %#v", orders)
	}
	if len(orders.Indexes) < 2 {
		t.Fatalf("index metadata incomplete: %#v", orders.Indexes)
	}
	created := findColumn(t, orders, "created_at")
	status := findColumn(t, orders, "status")
	if created.TypeCategory != TypeTemporal || created.Nullable {
		t.Fatalf("column metadata incomplete: %#v", created)
	}
	if status.TypeCategory != TypeText || len(status.AllowedValues) != 3 || status.AllowedValues[1] != "paid" {
		t.Fatalf("enum metadata incomplete: %#v", status)
	}
	compiled, err := CompilePostgres(one, RelationalIntent{
		Schema: RelationalIntentV1, SourceID: one.SourceID, SchemaFingerprint: one.Fingerprint,
		FromRelationID: orders.ID, Shape: ResultScalar,
		Projections: []RelationalProjection{{Aggregate: AggregateCountRows}}, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := ExecuteEvidence(ctx, pool, one, compiled, DefaultExecutionLimits())
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Result.RowCount != 1 || evidence.Result.Rows[0][0].Kind != EvidenceInteger || evidence.Result.Rows[0][0].Value != "2" {
		t.Fatalf("unexpected typed evidence: %#v", evidence)
	}
	again, err := ExecuteEvidence(ctx, pool, one, compiled, DefaultExecutionLimits())
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Result.Hash != again.Result.Hash {
		t.Fatal("identical database results produced different hashes")
	}
	recordFields := []string{"id", "amount", "created_at", "reference", "active", "note", "status"}
	recordProjections := make([]RelationalProjection, len(recordFields))
	for index, name := range recordFields {
		recordProjections[index] = RelationalProjection{FieldID: findColumn(t, orders, name).ID}
	}
	recordsQuery, err := CompilePostgres(one, RelationalIntent{
		Schema: RelationalIntentV1, SourceID: one.SourceID, SchemaFingerprint: one.Fingerprint,
		FromRelationID: orders.ID, Shape: ResultRecords, Projections: recordProjections,
		OrderBy: []OrderTerm{{Projection: 0, Direction: OrderAscending}}, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := ExecuteEvidence(ctx, pool, one, recordsQuery, DefaultExecutionLimits())
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := []EvidenceValueKind{EvidenceInteger, EvidenceDecimal, EvidenceTimestamp, EvidenceUUID, EvidenceBoolean, EvidenceNull, EvidenceText}
	if len(records.Result.Rows) != 2 || len(records.Result.Rows[0]) != len(wantKinds) {
		t.Fatalf("unexpected record evidence dimensions: %#v", records.Result)
	}
	for index, want := range wantKinds {
		if records.Result.Rows[0][index].Kind != want {
			t.Errorf("column %s kind=%q want=%q value=%q", recordFields[index], records.Result.Rows[0][index].Kind, want, records.Result.Rows[0][index].Value)
		}
	}
	tinyResult := DefaultExecutionLimits()
	tinyResult.MaxBytes = 1
	if _, err := ExecuteEvidence(ctx, pool, one, recordsQuery, tinyResult); err == nil {
		t.Fatal("typed evidence exceeding the byte bound was accepted")
	}
	lowCost := DefaultExecutionLimits()
	lowCost.MaxTotalCost = 0.0001
	if _, err := ExecuteEvidence(ctx, pool, one, compiled, lowCost); err == nil {
		t.Fatal("query exceeding the EXPLAIN cost limit executed")
	}
	if _, err := pool.Exec(ctx, "ALTER TABLE "+quotedSchema+`.orders ADD COLUMN description text`); err != nil {
		t.Fatal(err)
	}
	changed, err := InspectCatalog(ctx, pool, "integration-source", "Integration")
	if err != nil {
		t.Fatal(err)
	}
	if one.Fingerprint == changed.Fingerprint || orders.ID == relationByQualifiedName(t, changed, schema, "orders").ID {
		t.Fatal("catalog mutation did not invalidate snapshot identity")
	}
	if _, err := ExecuteEvidence(ctx, pool, one, compiled, DefaultExecutionLimits()); err == nil {
		t.Fatal("stale compiled query executed after a schema change")
	}
}

func relationByQualifiedName(t *testing.T, snapshot SchemaSnapshot, schema, name string) SchemaRelation {
	t.Helper()
	for _, relation := range snapshot.Relations {
		if relation.Schema == schema && relation.Name == name {
			return relation
		}
	}
	t.Fatalf("relation %s.%s not found", schema, name)
	return SchemaRelation{}
}
