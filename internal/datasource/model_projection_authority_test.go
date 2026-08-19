package datasource

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDatabaseCognitionProjectionHasNoDomainPromptCredentialRawSQLOrToolPath(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"schema_projection.go", "relational_intent_types.go", "relational_intent_decode.go", "evidence_types.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"DomainGuidance(", "ContextPrompt", "Password", "DSN", "RunSQL(",
			"tool_call", "run_sql", "search_web", "SELECT *",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s contains forbidden model-path authority %q", file, forbidden)
			}
		}
	}
	if _, err := os.Stat("profile.go"); !os.IsNotExist(err) {
		t.Fatalf("retired data-source profile authority still exists: %v", err)
	}
}

func TestCompiledPostgresQueryJSONOmitsSQLAndBoundValues(t *testing.T) {
	t.Parallel()
	snapshot, err := NewSchemaSnapshot("source-1", "Exact source", []RelationDefinition{{
		Schema: "public", Name: "records", Kind: RelationTable,
		Columns: []ColumnDefinition{
			{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger},
			{Name: "secret", Ordinal: 2, DataType: "text", TypeCategory: TypeText},
		},
	}}, time.Unix(1_700_000_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	relation := snapshot.Relations[0]
	compiled, err := CompilePostgres(snapshot, RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, FromRelationID: relation.ID,
		Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: relation.Columns[0].ID}},
		Filters: []RelationalPredicate{{
			FieldID: relation.Columns[1].ID, Operator: FilterEqual,
			Values: []IntentLiteral{{Type: LiteralString, Value: "credential-password"}},
		}}, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"credential-password", "select ", `from \"public\"`, `\"sql\"`, `\"value\"`,
	} {
		if strings.Contains(projection, forbidden) {
			t.Fatalf("compiled query JSON exposes SQL/bound value %q: %s", forbidden, encoded)
		}
	}
}
