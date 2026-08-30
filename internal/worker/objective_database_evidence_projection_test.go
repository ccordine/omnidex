package worker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestObjectiveDatabaseEvidenceSeparatesSemanticRowsFromCodeOwnedProvenance(t *testing.T) {
	t.Parallel()
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	resultHash := strings.Repeat("a", 64)
	acquiredAt := time.Unix(1_700_000_123, 0).UTC()
	intent := datasource.RelationalIntent{
		Schema: datasource.RelationalIntentV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, FromRelationID: snapshot.Relations[0].ID,
		Shape:       datasource.ResultScalar,
		Projections: []datasource.RelationalProjection{{Aggregate: datasource.AggregateCountRows}},
		Filters:     []datasource.RelationalPredicate{}, TemporalWindows: []datasource.TemporalWindow{},
		Exists: []datasource.ExistencePredicate{}, GroupBy: []int{},
		Having: []datasource.AggregatePredicate{}, OrderBy: []datasource.OrderTerm{}, Limit: 1,
	}
	evidence := datasource.EvidenceResult{
		Schema: datasource.EvidenceResultV1,
		Provenance: datasource.EvidenceProvenance{
			SourceID: snapshot.SourceID, SchemaFingerprint: snapshot.Fingerprint,
			IntentHash: strings.Repeat("b", 64), QueryHash: strings.Repeat("c", 64),
			ResultHash: resultHash,
			Plan:       datasource.ExecutionPlan{TotalCost: 9182.5, EstimatedRows: 77123},
			AcquiredAt: acquiredAt,
		},
		Result: datasource.TypedEvidenceResult{
			Columns: []datasource.EvidenceColumn{{
				Name: "result_name_hidden_sentinel", Aggregate: datasource.AggregateCountRows,
				TypeCategory: datasource.TypeInteger,
			}},
			Rows:     [][]datasource.EvidenceValue{{{Kind: datasource.EvidenceInteger, Value: "3"}}},
			RowCount: 1, Hash: resultHash,
		},
	}

	projected, err := projectObjectiveDatabaseEvidence(snapshot, intent, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 {
		t.Fatalf("database evidence capsules=%d", len(projected))
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(projected[0].Capsule.Text), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["columns"] == nil || payload["rows"] == nil {
		t.Fatalf("model evidence fields=%v text=%s", payload, projected[0].Capsule.Text)
	}
	for _, hidden := range []string{
		"schema", "provenance", "source_id", "schema_fingerprint", "intent_hash",
		"query_hash", "result_hash", "plan_cost", "estimated_rows", "returned_row_count",
		"row_offset", "result_name", "result_name_hidden_sentinel", snapshot.SourceID,
	} {
		if strings.Contains(projected[0].Capsule.Text, hidden) {
			t.Fatalf("model database evidence exposed %q: %s", hidden, projected[0].Capsule.Text)
		}
	}
	if !strings.Contains(projected[0].Capsule.Text, `"label":"count_rows"`) ||
		!strings.Contains(projected[0].Capsule.Text, `"value":"3"`) {
		t.Fatalf("model database evidence lost semantic result: %s", projected[0].Capsule.Text)
	}
	if projected[0].SourceSHA256 != resultHash || projected[0].ObservedAt != acquiredAt ||
		!strings.Contains(projected[0].SourceRef, "database:"+snapshot.SourceID) {
		t.Fatalf("code-owned provenance was not retained: %#v", projected[0])
	}
}
