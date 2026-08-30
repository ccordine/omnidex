package assemblyline

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseQueryIntentUsesRawSemanticLeavesAndRejectsJSON(t *testing.T) {
	input, _, _ := databaseQueryIntentFixture(t)
	state := NewDatabaseQueryIntentLeafState(input)
	state.FromRelationID = input.SchemaProjection.Relations[0].ID
	job, err := NewDatabaseQueryShapeJob(state)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, `{"shape"`) || !strings.Contains(prompt, "Return exactly one raw registered value") {
		t.Fatalf("query shape prompt is not a raw one-result contract: %s", prompt)
	}
	shape, err := DecodeDatabaseQueryShapeLeaf(state, "ranking")
	if err != nil || shape != datasource.ResultRanking {
		t.Fatalf("shape=%q err=%v", shape, err)
	}
	for _, raw := range []string{`{"shape":"ranking"}`, `["ranking"]`, `"ranking"`} {
		if _, err := DecodeDatabaseQueryShapeLeaf(state, raw); err == nil {
			t.Fatalf("database query shape accepted JSON %s", raw)
		}
	}
}

func TestDatabaseQueryIntentCodeAssemblesAndBindsTypedDecision(t *testing.T) {
	input, clinicID, appointmentClinicID := databaseQueryIntentFixture(t)
	decision, err := AssembleDatabaseQueryIntentDecision(input, DatabaseQueryIntentDecision{
		FromRelationID: input.SchemaProjection.Relations[0].ID,
		Shape:          datasource.ResultRanking,
		Projections: []datasource.RelationalProjection{
			{FieldID: appointmentClinicID},
			{Aggregate: datasource.AggregateCountRows},
		},
		Filters:         []datasource.RelationalPredicate{},
		TemporalWindows: []DatabaseTemporalWindowDecision{},
		Exists:          []datasource.ExistencePredicate{},
		GroupBy:         []int{0},
		Having:          []datasource.AggregatePredicate{},
		OrderBy:         []datasource.OrderTerm{{Projection: 1, Direction: datasource.OrderDescending}},
		Limit:           input.MaxRows,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent := decision.Bind(input)
	if intent.SourceID != "source-1" || intent.SchemaFingerprint != input.SchemaProjection.SchemaFingerprint || intent.Limit != input.MaxRows {
		t.Fatalf("bound intent=%+v", intent)
	}
	if clinicID == appointmentClinicID {
		t.Fatal("fixture column identities collapsed")
	}
}

func TestDatabaseQueryWindowLeafCannotAuthorTemporalClock(t *testing.T) {
	input, _, _ := databaseQueryIntentFixture(t)
	state := NewDatabaseQueryIntentLeafState(input)
	state.FromRelationID = input.SchemaProjection.Relations[0].ID
	state.Shape = datasource.ResultRecords
	createdAtID := ""
	for _, column := range input.SchemaProjection.Relations[0].Columns {
		if column.Name == "created_at" {
			createdAtID = column.ID
		}
	}
	leaf := DatabaseQueryWindowLeafInput{
		State: state, Purpose: "within the preceding ninety days",
		FieldID: createdAtID, Unit: datasource.WindowDay,
	}
	amount, err := DecodeDatabaseQueryWindowAmountLeaf(leaf, "90")
	if err != nil || amount != 90 {
		t.Fatalf("amount=%d err=%v", amount, err)
	}
	if _, err := DecodeDatabaseQueryWindowAmountLeaf(leaf, `{"amount":90,"as_of":"1999-01-01T00:00:00Z"}`); err == nil {
		t.Fatal("database window amount accepted model-authored clock JSON")
	}
	decision, err := AssembleDatabaseQueryIntentDecision(input, DatabaseQueryIntentDecision{
		FromRelationID: state.FromRelationID,
		Shape:          datasource.ResultRecords,
		Projections:    []datasource.RelationalProjection{{FieldID: createdAtID}},
		Filters:        []datasource.RelationalPredicate{},
		TemporalWindows: []DatabaseTemporalWindowDecision{{
			FieldID: createdAtID, Unit: datasource.WindowDay, Amount: amount,
		}},
		Exists:  []datasource.ExistencePredicate{},
		GroupBy: []int{}, Having: []datasource.AggregatePredicate{},
		OrderBy: []datasource.OrderTerm{}, Limit: input.MaxRows,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := decision.Bind(input).TemporalWindows[0].AsOf; got != input.TemporalAsOf {
		t.Fatalf("temporal authority=%q want=%q", got, input.TemporalAsOf)
	}
}

func TestDatabaseQueryIntentRejectsStringLiteralBeyondCodeOwnedByteCeiling(t *testing.T) {
	input, _, _ := databaseQueryIntentFixture(t)
	relation := input.SchemaProjection.Relations[0]
	textFieldID := ""
	for _, column := range relation.Columns {
		if column.TypeCategory == datasource.TypeText && len(column.AllowedValues) == 0 {
			textFieldID = column.ID
			break
		}
	}
	decision := DatabaseQueryIntentDecision{
		FromRelationID: relation.ID,
		Shape:          datasource.ResultRecords,
		Projections:    []datasource.RelationalProjection{{FieldID: textFieldID}},
		Filters: []datasource.RelationalPredicate{{
			FieldID: textFieldID, Operator: datasource.FilterEqual,
			Values: []datasource.IntentLiteral{{
				Type: datasource.LiteralString, Value: strings.Repeat("x", datasource.MaxIntentStringLiteralBytes+1),
			}},
		}},
		TemporalWindows: []DatabaseTemporalWindowDecision{},
		Exists:          []datasource.ExistencePredicate{},
		GroupBy:         []int{}, Having: []datasource.AggregatePredicate{},
		OrderBy: []datasource.OrderTerm{}, Limit: 1,
	}
	if _, err := AssembleDatabaseQueryIntentDecision(input, decision); err == nil {
		t.Fatalf("accepted string literal beyond %d bytes", datasource.MaxIntentStringLiteralBytes)
	}
}

func databaseQueryIntentFixture(t *testing.T) (DatabaseQueryIntentInput, string, string) {
	t.Helper()
	snapshot, err := datasource.NewSchemaSnapshot("source-1", "appointments", []datasource.RelationDefinition{
		{Schema: "public", Name: "clinics", Kind: datasource.RelationTable, Columns: []datasource.ColumnDefinition{
			{Name: "id", Ordinal: 1, DataType: "uuid", TypeCategory: datasource.TypeUUID},
			{Name: "name", Ordinal: 2, DataType: "text", TypeCategory: datasource.TypeText},
		}, PrimaryKey: []string{"id"}},
		{Schema: "public", Name: "appointments", Kind: datasource.RelationTable, Columns: []datasource.ColumnDefinition{
			{Name: "id", Ordinal: 1, DataType: "uuid", TypeCategory: datasource.TypeUUID},
			{Name: "clinic_id", Ordinal: 2, DataType: "uuid", TypeCategory: datasource.TypeUUID},
			{Name: "status", Ordinal: 3, DataType: "text", TypeCategory: datasource.TypeText},
			{Name: "created_at", Ordinal: 4, DataType: "timestamp with time zone", TypeCategory: datasource.TypeTemporal},
		}, PrimaryKey: []string{"id"}, ForeignKeys: []datasource.ForeignKeyDefinition{{
			Name: "appointments_clinic", Columns: []string{"clinic_id"}, ReferencedSchema: "public",
			ReferencedRelation: "clinics", ReferencedColumns: []string{"id"},
		}}},
	}, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	var clinics, appointments datasource.SchemaRelation
	for _, relation := range snapshot.Relations {
		switch relation.Name {
		case "clinics":
			clinics = relation
		case "appointments":
			appointments = relation
		}
	}
	projection, err := datasource.ProjectSchemaForIntent(snapshot, []string{appointments.ID, clinics.ID})
	if err != nil {
		t.Fatal(err)
	}
	return DatabaseQueryIntentInput{
		EvidenceNeedID: "need-2", ExactNeed: "Which clinics have the most appointments?",
		SchemaProjection: projection, TemporalAsOf: snapshot.CapturedAt.UTC().Format(time.RFC3339Nano), MaxRows: 50,
	}, clinics.Columns[0].ID, appointments.Columns[1].ID
}
