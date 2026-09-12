package assemblyline

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseSelectionExcludesConsumedChoicesWithoutRenderingHistory(t *testing.T) {
	state := databaseSelectionStateFixture("shipments", "weight", "dispatched_at")
	order := DatabaseQueryOrderLeafInput{State: state, Purpose: "Order by weight."}
	existence := DatabaseQueryExistenceLeafInput{
		State: state, Purpose: "Matching remaining records must exist.", Filters: []datasource.RelationalPredicate{},
	}
	orderPrompt, err := BuildDatabaseQueryOrderProjectionPrompt(order)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(orderPrompt, "unrelated_measure") || strings.Count(orderPrompt, "public.shipments.weight") != 1 {
		t.Fatalf("ordering must show each remaining projection once, without the consumed projection: %s", orderPrompt)
	}
	existencePrompt, err := BuildDatabaseQueryExistenceRelationPrompt(existence)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(existencePrompt, "public.unrelated_records") {
		t.Fatalf("existence selection exposed an already consumed relation: %s", existencePrompt)
	}
	order.State.OrderBy = append(order.State.OrderBy, datasource.OrderTerm{Projection: 0, Direction: datasource.OrderAscending})
	projection, resolved, err := ResolveSoleDatabaseQueryOrderProjectionLeaf(order)
	if err != nil || !resolved || projection != 2 {
		t.Fatalf("remaining projection was not resolved by code: %d %t %v", projection, resolved, err)
	}
	if _, err := BuildDatabaseQueryOrderProjectionPrompt(order); err == nil {
		t.Fatal("sole remaining projection was rendered for a model")
	}
	existence.State.Exists = append(existence.State.Exists,
		datasource.ExistencePredicate{RelationID: "focus", Filters: []datasource.RelationalPredicate{}})
	relation, resolved, err := ResolveSoleDatabaseQueryExistenceRelationLeaf(existence)
	if err != nil || !resolved || relation != "remaining" {
		t.Fatalf("remaining relation was not resolved by code: %q %t %v", relation, resolved, err)
	}
	if _, err := BuildDatabaseQueryExistenceRelationPrompt(existence); err == nil {
		t.Fatal("sole remaining relation was rendered for a model")
	}
	order.State.OrderBy = append(order.State.OrderBy, datasource.OrderTerm{Projection: 2, Direction: datasource.OrderAscending})
	if _, err := BuildDatabaseQueryOrderProjectionPrompt(order); err == nil {
		t.Fatal("exhausted projection choices did not fail before model rendering")
	}
	if _, err := DecodeDatabaseQueryOrderProjectionLeaf(order, "A"); err == nil {
		t.Fatal("exhausted projection choices accepted a model result")
	}
	existence.State.Exists = append(existence.State.Exists,
		datasource.ExistencePredicate{RelationID: "remaining", Filters: []datasource.RelationalPredicate{}})
	if _, err := BuildDatabaseQueryExistenceRelationPrompt(existence); err == nil {
		t.Fatal("exhausted relation choices did not fail before model rendering")
	}
	if _, err := DecodeDatabaseQueryExistenceRelationLeaf(existence, "A"); err == nil {
		t.Fatal("exhausted relation choices accepted a model result")
	}
}

func TestDatabaseScopedFieldSelectionKeepsOnlyItsScopeAndParentPurpose(t *testing.T) {
	state := databaseSelectionStateFixture("shipments", "weight", "dispatched_at")
	state.Authority.SchemaProjection.Relations[2].Columns = append(state.Authority.SchemaProjection.Relations[2].Columns,
		datasource.IntentColumnProjection{ID: "remaining-state", Name: "state", TypeCategory: datasource.TypeText, AllowedValues: []string{"open", "closed"}})
	input := DatabaseQueryFilterLeafInput{
		State: state, ScopeRelationID: "remaining", Purpose: "Require the open state.",
		ParentPurpose: "Match the requested related records.", AcceptedValues: []datasource.IntentLiteral{},
		AcceptedFilters: []datasource.RelationalPredicate{{FieldID: "remaining-id", Operator: datasource.FilterIsNotNull}},
	}
	prompt, err := BuildDatabaseQueryFilterFieldPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{input.Purpose, input.ParentPurpose, "public.remaining_records", "open", "closed"} {
		if !strings.Contains(prompt, required) {
			t.Errorf("scoped field selection omitted necessary meaning %q", required)
		}
	}
	for _, hidden := range []string{"public.shipments", "public.unrelated_records", "ACCEPTED FILTERS", "Has a value"} {
		if strings.Contains(prompt, hidden) {
			t.Errorf("scoped field selection exposed unrelated authority %q", hidden)
		}
	}
	field, err := DecodeDatabaseQueryFilterFieldLeaf(input, "B")
	if err != nil || field != "remaining-state" {
		t.Fatalf("scoped field selection = %q, %v", field, err)
	}
}

func TestDatabaseSelectionHasNoAcceptedClauseRenderingPath(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, retired := range []string{
			"renderDatabaseQueryAcceptedProjections", "renderDatabaseQueryAcceptedFilters",
			"renderDatabaseQueryAcceptedWindows", "renderDatabaseQueryAcceptedExistence",
			"renderDatabaseQueryAcceptedHaving", "renderDatabaseQueryAcceptedOrder",
		} {
			if strings.Contains(string(source), retired) {
				t.Errorf("%s retains the removed full-clause model-context path %s", entry.Name(), retired)
			}
		}
	}
}
