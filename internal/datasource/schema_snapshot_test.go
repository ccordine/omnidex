package datasource

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewSchemaSnapshotCanonicalizesMetadataAndScopesOpaqueIDs(t *testing.T) {
	t.Parallel()
	captured := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	definitions := []RelationDefinition{
		{
			Schema: "public", Name: "orders", Kind: RelationTable,
			Columns: []ColumnDefinition{
				{Name: "customer_id", Ordinal: 2, DataType: "bigint", TypeCategory: TypeInteger, Nullable: false},
				{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger, Nullable: false},
			},
			PrimaryKey: []string{"id"},
			ForeignKeys: []ForeignKeyDefinition{{
				Name: "orders_customer_fk", Columns: []string{"customer_id"},
				ReferencedSchema: "public", ReferencedRelation: "customers", ReferencedColumns: []string{"id"},
			}},
			Indexes: []IndexDefinition{{Name: "orders_customer_idx", Columns: []string{"customer_id"}}},
		},
		{
			Schema: "public", Name: "customers", Kind: RelationTable,
			Columns:    []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}},
			PrimaryKey: []string{"id"},
		},
	}

	one, err := NewSchemaSnapshot("source-a", "Commerce", definitions, captured)
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewSchemaSnapshot("source-a", "Renamed presentation", []RelationDefinition{definitions[1], definitions[0]}, captured.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if one.Fingerprint != two.Fingerprint {
		t.Fatalf("fingerprints differ for identical schema: %q != %q", one.Fingerprint, two.Fingerprint)
	}
	if one.Relations[0].Name != "customers" || one.Relations[1].Columns[0].Name != "id" {
		t.Fatalf("snapshot is not canonical: %#v", one.Relations)
	}
	if one.Relations[0].ID != two.Relations[0].ID || one.Relations[0].Columns[0].ID != two.Relations[0].Columns[0].ID {
		t.Fatal("opaque IDs changed without an authoritative schema change")
	}
	otherSource, err := NewSchemaSnapshot("source-b", "Commerce", definitions, captured)
	if err != nil {
		t.Fatal(err)
	}
	if one.Relations[0].ID == otherSource.Relations[0].ID {
		t.Fatal("opaque relation ID was not scoped to the source")
	}
	changed := append([]RelationDefinition(nil), definitions...)
	changed[0] = definitions[0]
	changed[0].Columns = append([]ColumnDefinition(nil), definitions[0].Columns...)
	changed[0].Columns = append(changed[0].Columns, ColumnDefinition{Name: "status", Ordinal: 3, DataType: "text", TypeCategory: TypeText})
	three, err := NewSchemaSnapshot("source-a", "Commerce", changed, captured)
	if err != nil {
		t.Fatal(err)
	}
	if one.Fingerprint == three.Fingerprint || one.Relations[0].ID == three.Relations[0].ID {
		t.Fatal("schema mutation did not invalidate fingerprint-scoped opaque IDs")
	}
	orders := one.Relations[1]
	if len(orders.ForeignKeys) != 1 || orders.ForeignKeys[0].ReferencedRelationID != one.Relations[0].ID {
		t.Fatalf("foreign key was not resolved to opaque IDs: %#v", orders.ForeignKeys)
	}
}

func TestNewSchemaSnapshotRejectsBrokenOrDuplicateMetadata(t *testing.T) {
	t.Parallel()
	cases := map[string][]RelationDefinition{
		"duplicate relation": {
			{Schema: "public", Name: "items", Kind: RelationTable, Columns: []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}}},
			{Schema: "public", Name: "items", Kind: RelationView, Columns: []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}}},
		},
		"duplicate column": {{
			Schema: "public", Name: "items", Kind: RelationTable,
			Columns: []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}, {Name: "id", Ordinal: 2, DataType: "text", TypeCategory: TypeText}},
		}},
		"broken foreign key": {{
			Schema: "public", Name: "items", Kind: RelationTable,
			Columns:     []ColumnDefinition{{Name: "owner_id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}},
			ForeignKeys: []ForeignKeyDefinition{{Name: "broken", Columns: []string{"owner_id"}, ReferencedSchema: "public", ReferencedRelation: "missing", ReferencedColumns: []string{"id"}}},
		}},
	}
	for name, definitions := range cases {
		definitions := definitions
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewSchemaSnapshot("source", "name", definitions, time.Now()); err == nil {
				t.Fatal("invalid schema metadata was accepted")
			}
		})
	}
}

func TestSchemaFingerprintExcludesVolatileEstimatesAndPreservesCompositeKeyOrder(t *testing.T) {
	t.Parallel()
	definition := RelationDefinition{
		Schema: "public", Name: "memberships", Kind: RelationTable, RowEstimate: 10,
		Columns: []ColumnDefinition{
			{Name: "account_id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger},
			{Name: "member_id", Ordinal: 2, DataType: "bigint", TypeCategory: TypeInteger},
		},
		PrimaryKey: []string{"member_id", "account_id"},
	}
	one, err := NewSchemaSnapshot("source", "Source", []RelationDefinition{definition}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	definition.RowEstimate = 999999
	two, err := NewSchemaSnapshot("source", "Source", []RelationDefinition{definition}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if one.Fingerprint != two.Fingerprint || one.Relations[0].ID != two.Relations[0].ID {
		t.Fatal("volatile planner row estimate changed schema identity")
	}
	member := findColumn(t, one.Relations[0], "member_id")
	account := findColumn(t, one.Relations[0], "account_id")
	if len(one.Relations[0].PrimaryKey) != 2 || one.Relations[0].PrimaryKey[0] != member.ID || one.Relations[0].PrimaryKey[1] != account.ID {
		t.Fatalf("composite primary-key order was changed: %#v", one.Relations[0].PrimaryKey)
	}
}

func TestSchemaSnapshotIntegrityRejectsMetadataOrOpaqueIDTampering(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored SchemaSnapshot
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.ValidateIntegrity(); err != nil {
		t.Fatalf("canonical persisted snapshot failed integrity: %v", err)
	}
	metadataTamper := restored
	metadataTamper.Relations = append([]SchemaRelation(nil), restored.Relations...)
	metadataTamper.Relations[0].Name = "renamed_without_fingerprint"
	if err := metadataTamper.ValidateIntegrity(); err == nil {
		t.Fatal("schema metadata tampering was accepted")
	}
	idTamper := restored
	idTamper.Relations = append([]SchemaRelation(nil), restored.Relations...)
	idTamper.Relations[0].ID = "rel_forged"
	if err := idTamper.ValidateIntegrity(); err == nil {
		t.Fatal("opaque relation ID tampering was accepted")
	}
}
