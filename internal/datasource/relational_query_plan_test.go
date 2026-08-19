package datasource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRelationalQueryPlanCarriesTypedAuthorityWithoutSQL(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders := findRelation(t, snapshot, "orders")
	customers := findRelation(t, snapshot, "customers")
	name := findColumn(t, customers, "name")
	intent := RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, FromRelationID: orders.ID,
		Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: name.ID}}, Limit: 10,
	}
	plan, err := BuildRelationalQueryPlan(snapshot, intent, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(snapshot); err != nil {
		t.Fatal(err)
	}
	if len(plan.JoinPaths) != 1 || plan.JoinPaths[0].TargetRelationID != customers.ID ||
		len(plan.JoinPaths[0].Path.Steps) != 1 {
		t.Fatalf("plan did not carry the exact deterministic join path: %+v", plan.JoinPaths)
	}
	if len(plan.Outputs) != 1 || plan.Outputs[0].FieldID != name.ID || plan.Outputs[0].Name != "c1" {
		t.Fatalf("plan outputs=%+v", plan.Outputs)
	}
	compiled, err := CompilePostgresPlan(snapshot, plan)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := CompilePostgres(snapshot, intent)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.QueryHash != direct.QueryHash || compiled.IntentHash != plan.IntentHash {
		t.Fatalf("compiled plan diverged from direct compilation: plan=%+v direct=%+v", compiled, direct)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	visible := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"select ", "from ", "parameter", "password", "credential"} {
		if strings.Contains(visible, forbidden) {
			t.Fatalf("delegated relational plan leaked %q: %s", forbidden, visible)
		}
	}
}

func TestRelationalQueryPlanPreservesOnlyRequiredSemanticJoinSelection(t *testing.T) {
	t.Parallel()
	snapshot, messages, people, intent := ambiguousRelationalPlanFixture(t)
	_, err := BuildRelationalQueryPlan(snapshot, intent, nil)
	if err == nil {
		t.Fatal("ambiguous relation plan was accepted without one semantic selection")
	}
	var selected string
	for _, foreignKey := range messages.ForeignKeys {
		if foreignKey.Name != "sender" {
			continue
		}
		for _, candidate := range err.(*AmbiguousJoinPathError).Candidates {
			if candidate.Steps[0].ForeignKeyID == foreignKey.ID {
				selected = candidate.ID
			}
		}
	}
	plan, err := BuildRelationalQueryPlan(snapshot, intent, map[string]string{people.ID: selected})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.JoinPathSelections) != 1 || plan.JoinPathSelections[0].TargetRelationID != people.ID ||
		plan.JoinPathSelections[0].PathID != selected {
		t.Fatalf("semantic join selection=%+v", plan.JoinPathSelections)
	}
	if len(plan.JoinPaths) != 1 || plan.JoinPaths[0].Path.ID != selected {
		t.Fatalf("resolved join paths=%+v", plan.JoinPaths)
	}
}

func TestRelationalQueryPlanRejectsTampering(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders := findRelation(t, snapshot, "orders")
	id := findColumn(t, orders, "id")
	plan, err := BuildRelationalQueryPlan(snapshot, RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, FromRelationID: orders.ID,
		Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: id.ID}}, Limit: 5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RelationalQueryPlan){
		"schema":      func(value *RelationalQueryPlan) { value.Schema = "invented" },
		"intent":      func(value *RelationalQueryPlan) { value.Intent.Limit++ },
		"intent hash": func(value *RelationalQueryPlan) { value.IntentHash = strings.Repeat("a", 64) },
		"plan hash":   func(value *RelationalQueryPlan) { value.PlanHash = strings.Repeat("b", 64) },
		"output":      func(value *RelationalQueryPlan) { value.Outputs[0].Name = "raw" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneRelationalQueryPlan(t, plan)
			mutate(&candidate)
			if err := candidate.Validate(snapshot); err == nil {
				t.Fatal("tampered delegated plan was accepted")
			}
		})
	}
}

func TestEvidenceResultValidationBindsAuthorizedPlanAndRows(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders := findRelation(t, snapshot, "orders")
	id := findColumn(t, orders, "id")
	plan, err := BuildRelationalQueryPlan(snapshot, RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, FromRelationID: orders.ID,
		Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: id.ID}}, Limit: 5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	columns := []EvidenceColumn{{
		Name: "c1", PostgresTypeOID: postgresOIDInt8, FieldID: id.ID, TypeCategory: TypeInteger,
	}}
	rows := [][]EvidenceValue{{{Kind: EvidenceInteger, Value: "41"}}}
	columnBytes, _ := json.Marshal(columns)
	rowBytes, _ := json.Marshal(rows[0])
	result, err := finalizeTypedResult(columns, rows, len(columnBytes)+len(rowBytes))
	if err != nil {
		t.Fatal(err)
	}
	queryDigest := sha256.Sum256([]byte("host-authorized-query"))
	evidence := EvidenceResult{
		Schema: EvidenceResultV1,
		Provenance: EvidenceProvenance{
			SourceID: snapshot.SourceID, SchemaFingerprint: snapshot.Fingerprint,
			IntentHash: plan.IntentHash, QueryHash: hex.EncodeToString(queryDigest[:]),
			ResultHash: result.Hash, Plan: ExecutionPlan{TotalCost: 2, EstimatedRows: 1},
			AcquiredAt: time.Unix(1_700_000_100, 0).UTC(),
		},
		Result: result,
	}
	limits := DefaultExecutionLimits()
	limits.MaxRows = 5
	if err := evidence.ValidateForPlan(snapshot, plan, limits); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*EvidenceResult){
		"intent": func(value *EvidenceResult) { value.Provenance.IntentHash = strings.Repeat("a", 64) },
		"rows":   func(value *EvidenceResult) { value.Result.Rows[0][0].Value = "42" },
		"field":  func(value *EvidenceResult) { value.Result.Columns[0].FieldID = "col_unbound" },
		"cost":   func(value *EvidenceResult) { value.Provenance.Plan.TotalCost = limits.MaxTotalCost + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := evidence
			candidate.Result.Columns = append([]EvidenceColumn(nil), evidence.Result.Columns...)
			candidate.Result.Rows = cloneEvidenceRows(evidence.Result.Rows)
			mutate(&candidate)
			if err := candidate.ValidateForPlan(snapshot, plan, limits); err == nil {
				t.Fatal("forged delegated evidence was accepted")
			}
		})
	}
}

func ambiguousRelationalPlanFixture(t *testing.T) (SchemaSnapshot, SchemaRelation, SchemaRelation, RelationalIntent) {
	t.Helper()
	snapshot, err := NewSchemaSnapshot("ambiguous-plan", "Ambiguous plan", []RelationDefinition{
		{Schema: "public", Name: "people", Kind: RelationTable, Columns: []ColumnDefinition{
			{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger},
			{Name: "name", Ordinal: 2, DataType: "text", TypeCategory: TypeText},
		}, PrimaryKey: []string{"id"}},
		{Schema: "public", Name: "messages", Kind: RelationTable, Columns: []ColumnDefinition{
			{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger},
			{Name: "sender_id", Ordinal: 2, DataType: "bigint", TypeCategory: TypeInteger},
			{Name: "recipient_id", Ordinal: 3, DataType: "bigint", TypeCategory: TypeInteger},
		}, ForeignKeys: []ForeignKeyDefinition{
			{Name: "sender", Columns: []string{"sender_id"}, ReferencedSchema: "public", ReferencedRelation: "people", ReferencedColumns: []string{"id"}},
			{Name: "recipient", Columns: []string{"recipient_id"}, ReferencedSchema: "public", ReferencedRelation: "people", ReferencedColumns: []string{"id"}},
		}},
	}, time.Unix(1_700_000_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	messages, people := findRelation(t, snapshot, "messages"), findRelation(t, snapshot, "people")
	name := findColumn(t, people, "name")
	intent := RelationalIntent{
		Schema: RelationalIntentV1, SourceID: snapshot.SourceID,
		SchemaFingerprint: snapshot.Fingerprint, FromRelationID: messages.ID,
		Shape: ResultRecords, Projections: []RelationalProjection{{FieldID: name.ID}}, Limit: 10,
	}
	return snapshot, messages, people, intent
}

func cloneRelationalQueryPlan(t *testing.T, plan RelationalQueryPlan) RelationalQueryPlan {
	t.Helper()
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var cloned RelationalQueryPlan
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(cloned, RelationalQueryPlan{}) {
		t.Fatal("cloned plan is empty")
	}
	return cloned
}
