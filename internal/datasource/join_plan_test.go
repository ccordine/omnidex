package datasource

import (
	"errors"
	"testing"
	"time"
)

func TestPlanJoinPathReturnsUniqueDeterministicForeignKeyPath(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	orders := findRelation(t, snapshot, "orders")
	customers := findRelation(t, snapshot, "customers")
	path, err := PlanJoinPath(snapshot, orders.ID, customers.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(path.Steps) != 1 || path.Steps[0].ForeignKeyID != orders.ForeignKeys[0].ID || path.Steps[0].Direction != JoinAlongForeignKey {
		t.Fatalf("unexpected path: %#v", path)
	}
	reverse, err := PlanJoinPath(snapshot, customers.ID, orders.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reverse.Steps) != 1 || reverse.Steps[0].Direction != JoinAgainstForeignKey {
		t.Fatalf("unexpected reverse path: %#v", reverse)
	}
}

func TestPlanJoinPathReturnsOpaqueCandidatesForAmbiguousShortestPaths(t *testing.T) {
	t.Parallel()
	snapshot, err := NewSchemaSnapshot("ambiguous", "Ambiguous", []RelationDefinition{
		{Schema: "public", Name: "people", Kind: RelationTable, Columns: []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}}, PrimaryKey: []string{"id"}},
		{Schema: "public", Name: "messages", Kind: RelationTable, Columns: []ColumnDefinition{
			{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger},
			{Name: "sender_id", Ordinal: 2, DataType: "bigint", TypeCategory: TypeInteger},
			{Name: "recipient_id", Ordinal: 3, DataType: "bigint", TypeCategory: TypeInteger},
		}, PrimaryKey: []string{"id"}, ForeignKeys: []ForeignKeyDefinition{
			{Name: "messages_sender_fk", Columns: []string{"sender_id"}, ReferencedSchema: "public", ReferencedRelation: "people", ReferencedColumns: []string{"id"}},
			{Name: "messages_recipient_fk", Columns: []string{"recipient_id"}, ReferencedSchema: "public", ReferencedRelation: "people", ReferencedColumns: []string{"id"}},
		}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	messages, people := findRelation(t, snapshot, "messages"), findRelation(t, snapshot, "people")
	_, err = PlanJoinPath(snapshot, messages.ID, people.ID)
	var ambiguous *AmbiguousJoinPathError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected ambiguity, got %v", err)
	}
	if len(ambiguous.Candidates) != 2 || ambiguous.Candidates[0].ID == ambiguous.Candidates[1].ID || ambiguous.Candidates[0].ID == "" {
		t.Fatalf("invalid opaque candidates: %#v", ambiguous.Candidates)
	}
}

func TestPlanJoinPathFailsWhenNoForeignKeyRouteExists(t *testing.T) {
	t.Parallel()
	snapshot := commerceSnapshot(t)
	disconnected, err := NewSchemaSnapshot("disconnected", "Disconnected", []RelationDefinition{
		{Schema: "public", Name: "a", Kind: RelationTable, Columns: []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}}},
		{Schema: "public", Name: "b", Kind: RelationTable, Columns: []ColumnDefinition{{Name: "id", Ordinal: 1, DataType: "bigint", TypeCategory: TypeInteger}}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PlanJoinPath(disconnected, disconnected.Relations[0].ID, disconnected.Relations[1].ID); err == nil {
		t.Fatal("disconnected relations produced a join path")
	}
	if _, err := PlanJoinPath(snapshot, "rel_unknown", snapshot.Relations[0].ID); err == nil {
		t.Fatal("unknown relation ID produced a join path")
	}
}
