package datasource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeExecutionPlanReadsOnlyTopLevelPlannerEvidence(t *testing.T) {
	t.Parallel()
	plan, err := decodeExecutionPlan([]byte(`[{
  "Plan":{"Node Type":"Limit","Plan Rows":17,"Total Cost":42.75,"Plans":[{"Plan Rows":9000,"Total Cost":4000}]}
}]`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.TotalCost != 42.75 || plan.EstimatedRows != 17 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	for _, invalid := range [][]byte{nil, []byte(`[]`), []byte(`not-json`)} {
		if _, err := decodeExecutionPlan(invalid); err == nil {
			t.Fatalf("invalid plan %q was accepted", invalid)
		}
	}
}

func TestTypedEvidenceValuesAndHashAreDeterministic(t *testing.T) {
	t.Parallel()
	instant := time.Date(2026, 8, 18, 12, 3, 4, 500, time.FixedZone("offset", -4*60*60))
	tests := []struct {
		oid   uint32
		value any
		want  EvidenceValue
	}{
		{postgresOIDBool, true, EvidenceValue{Kind: EvidenceBoolean, Value: "true"}},
		{postgresOIDInt8, int64(9), EvidenceValue{Kind: EvidenceInteger, Value: "9"}},
		{postgresOIDNumeric, "10.50", EvidenceValue{Kind: EvidenceDecimal, Value: "10.50"}},
		{postgresOIDTimestamptz, instant, EvidenceValue{Kind: EvidenceTimestamp, Value: instant.UTC().Format(time.RFC3339Nano)}},
		{postgresOIDJSONB, []byte(`{"b":2,"a":1}`), EvidenceValue{Kind: EvidenceJSON, Value: `{"a":1,"b":2}`}},
		{postgresOIDBytea, []byte{0, 1, 2}, EvidenceValue{Kind: EvidenceBinary, Value: "AAEC"}},
		{postgresOIDText, nil, EvidenceValue{Kind: EvidenceNull}},
	}
	row := make([]EvidenceValue, len(tests))
	for index, test := range tests {
		value, err := normalizeEvidenceValue(test.oid, test.value)
		if err != nil {
			t.Fatal(err)
		}
		if value != test.want {
			t.Fatalf("value %d=%#v want=%#v", index, value, test.want)
		}
		row[index] = value
	}
	columns := []EvidenceColumn{{Name: "c1", PostgresTypeOID: postgresOIDBool, TypeCategory: TypeBoolean}}
	one, err := finalizeTypedResult(columns, [][]EvidenceValue{row}, 100)
	if err != nil {
		t.Fatal(err)
	}
	two, err := finalizeTypedResult(columns, [][]EvidenceValue{row}, 999)
	if err != nil {
		t.Fatal(err)
	}
	if one.Hash == "" || one.Hash != two.Hash {
		t.Fatalf("result hash depends on non-result metadata: %q != %q", one.Hash, two.Hash)
	}
}

func TestExecuteEvidenceRejectsForgedRawSQLBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders := findRelation(t, snapshot, "orders")
	id := findColumn(t, orders, "id")
	compiled, err := CompilePostgres(snapshot, RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID, SchemaFingerprint: snapshot.Fingerprint,
		FromRelationID: orders.ID, Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: id.ID}}, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	forged := compiled
	forged.SQL = `DELETE FROM "public"."orders"`
	digest := sha256.Sum256([]byte(forged.SQL))
	forged.QueryHash = hex.EncodeToString(digest[:])
	if _, err := ExecuteEvidence(t.Context(), nil, snapshot, forged, DefaultExecutionLimits()); err == nil || !strings.Contains(err.Error(), "not sealed") {
		t.Fatalf("forged raw SQL did not fail at compiler seal: %v", err)
	}
}

func TestEvidenceProjectionContainsResultsAndProvenanceWithoutSQLOrParameters(t *testing.T) {
	t.Parallel()
	evidence := EvidenceResult{
		Schema:     EvidenceResultV1,
		Provenance: EvidenceProvenance{SourceID: "source", SchemaFingerprint: "fingerprint", IntentHash: "intent", QueryHash: "query", ResultHash: "result"},
		Result:     TypedEvidenceResult{Columns: []EvidenceColumn{{Name: "c1", TypeCategory: TypeInteger}}, Rows: [][]EvidenceValue{{{Kind: EvidenceInteger, Value: "4"}}}, RowCount: 1, Hash: "result"},
	}
	encoded, err := json.Marshal(evidence.Projection())
	if err != nil {
		t.Fatal(err)
	}
	visible := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"sql", "parameter", "password", "tool", "execute", "context_prompt"} {
		if strings.Contains(visible, forbidden) {
			t.Errorf("evidence projection contains forbidden %q: %s", forbidden, visible)
		}
	}
}
