package assemblyline

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseQueryIntentBindsSemanticLeafToCodeOwnedSnapshot(t *testing.T) {
	input, clinicID, appointmentClinicID := databaseQueryIntentFixture(t)
	raw := `{"schema":"omnidex.database-query-intent.v1","evidence_need_id":"need-2","from_relation_id":"` + input.SchemaProjection.Relations[0].ID + `","shape":"ranking","projections":[{"field_id":"` + appointmentClinicID + `","aggregate":"","time_bucket":""},{"field_id":"","aggregate":"count_rows","time_bucket":""}],"filters":[],"temporal_windows":[],"exists":[],"group_by":[0],"having":[],"order_by":[{"projection":1,"direction":"desc"}],"limit":20}`
	decision, err := DecodeDatabaseQueryIntentDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	intent := decision.Bind(input)
	if intent.SourceID != "source-1" || intent.SchemaFingerprint != input.SchemaProjection.SchemaFingerprint || intent.Limit != 20 {
		t.Fatalf("bound intent=%+v", intent)
	}
	if clinicID == appointmentClinicID {
		t.Fatal("fixture column identities collapsed")
	}
	job, err := NewDatabaseQueryIntentJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password", "dsn", "run_sql", "search_web"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("model projection contains forbidden %q", forbidden)
		}
	}
}

func TestDatabaseQueryIntentRejectsSQLControlAndUnknownSchemaIDs(t *testing.T) {
	input, _, appointmentClinicID := databaseQueryIntentFixture(t)
	base := `{"schema":"omnidex.database-query-intent.v1","evidence_need_id":"need-2","from_relation_id":"%s","shape":"records","projections":[{"field_id":"%s","aggregate":"","time_bucket":""}],"filters":[],"temporal_windows":[],"exists":[],"group_by":[],"having":[],"order_by":[],"limit":20}`
	valid := fmt.Sprintf(base, input.SchemaProjection.Relations[0].ID, appointmentClinicID)
	if _, err := DecodeDatabaseQueryIntentDecision(input, valid); err != nil {
		t.Fatalf("valid fixture: %v\n%s", err, valid)
	}
	for name, raw := range map[string]string{
		"sql":       strings.TrimSuffix(valid, "}") + `,"sql":"SELECT *"}`,
		"operation": strings.TrimSuffix(valid, "}") + `,"execute":true}`,
		"relation":  strings.Replace(valid, input.SchemaProjection.Relations[0].ID, "rel_invented", 1),
		"column":    strings.Replace(valid, appointmentClinicID, "col_invented", 1),
		"limit":     strings.Replace(valid, `"limit":20`, `"limit":51`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeDatabaseQueryIntentDecision(input, raw); err == nil {
				t.Fatalf("accepted invalid candidate: %s", raw)
			}
		})
	}
}

func TestDatabaseQueryIntentResponseSchemaPermitsCodeValidatedExistenceShape(t *testing.T) {
	input, _, _ := databaseQueryIntentFixture(t)
	schema, err := DatabaseQueryIntentResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("response schema properties=%T", schema["properties"])
	}
	projections, ok := properties["projections"].(map[string]any)
	if !ok || projections["minItems"] != 0 {
		t.Fatalf("projection schema=%v, want minItems=0 for existence", projections)
	}
	filters := properties["filters"].(map[string]any)
	filter := filters["items"].(map[string]any)
	filterProperties := filter["properties"].(map[string]any)
	values := filterProperties["values"].(map[string]any)
	literal := values["items"].(map[string]any)
	literalProperties := literal["properties"].(map[string]any)
	literalValue := literalProperties["value"].(map[string]any)
	if _, finiteGrammarBound := literalValue["maxLength"]; finiteGrammarBound {
		t.Fatalf("database literal schema encodes the code-owned byte ceiling: %#v", literalValue)
	}
	existence := `{"schema":"omnidex.database-query-intent.v1","evidence_need_id":"need-2","from_relation_id":"` + input.SchemaProjection.Relations[0].ID + `","shape":"existence","projections":[],"filters":[],"temporal_windows":[],"exists":[],"group_by":[],"having":[],"order_by":[],"limit":1}`
	if _, err := DecodeDatabaseQueryIntentDecision(input, existence); err != nil {
		t.Fatalf("existence intent rejected: %v", err)
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
	if textFieldID == "" {
		t.Fatal("fixture omitted an unrestricted text field")
	}
	decision := DatabaseQueryIntentDecision{
		Schema: DatabaseQueryIntentV1, EvidenceNeedID: input.EvidenceNeedID,
		FromRelationID: relation.ID, Shape: datasource.ResultRecords,
		Projections: []datasource.RelationalProjection{{FieldID: textFieldID}},
		Filters: []datasource.RelationalPredicate{{
			FieldID: textFieldID, Operator: datasource.FilterEqual,
			Values: []datasource.IntentLiteral{{
				Type:  datasource.LiteralString,
				Value: strings.Repeat("x", datasource.MaxIntentStringLiteralBytes+1),
			}},
		}},
		Limit: 1,
	}
	if err := decision.ValidateFor(input); err == nil {
		t.Fatalf(
			"database query intent accepted a string literal beyond %d bytes",
			datasource.MaxIntentStringLiteralBytes,
		)
	}
}

func TestDatabaseQueryIntentBindsServerTemporalAuthority(t *testing.T) {
	input, _, _ := databaseQueryIntentFixture(t)
	relation := input.SchemaProjection.Relations[0]
	createdAtID := ""
	for _, column := range relation.Columns {
		if column.Name == "created_at" {
			createdAtID = column.ID
		}
	}
	if createdAtID == "" {
		t.Fatal("fixture omitted temporal field")
	}
	raw := `{"schema":"omnidex.database-query-intent.v1","evidence_need_id":"need-2","from_relation_id":"` + relation.ID + `","shape":"records","projections":[{"field_id":"` + createdAtID + `","aggregate":"","time_bucket":""}],"filters":[],"temporal_windows":[{"field_id":"` + createdAtID + `","unit":"day","amount":90}],"exists":[],"group_by":[],"having":[],"order_by":[],"limit":20}`
	decision, err := DecodeDatabaseQueryIntentDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	intent := decision.Bind(input)
	if len(intent.TemporalWindows) != 1 || intent.TemporalWindows[0].AsOf != input.TemporalAsOf {
		t.Fatalf("temporal authority=%+v want=%q", intent.TemporalWindows, input.TemporalAsOf)
	}
	modelAuthoredClock := strings.Replace(raw, `"amount":90}`, `"amount":90,"as_of":"1999-01-01T00:00:00Z"}`, 1)
	if _, err := DecodeDatabaseQueryIntentDecision(input, modelAuthoredClock); err == nil {
		t.Fatal("model-authored temporal authority was accepted")
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
	clinicID := clinics.Columns[0].ID
	appointmentClinicID := appointments.Columns[1].ID
	return DatabaseQueryIntentInput{
		EvidenceNeedID: "need-2", ExactNeed: "Which clinics have the most appointments?",
		SchemaProjection: projection,
		TemporalAsOf:     snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
		MaxRows:          50,
	}, clinicID, appointmentClinicID
}
