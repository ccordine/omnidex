package worker

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func TestObjectiveDatabaseSchemaSelectionUsesExactFixedPointLeafCalls(t *testing.T) {
	input := assemblyline.DatabaseSchemaSelectionInput{
		EvidenceNeedID: "need-schema", ExactNeed: "Which relation contains the answer?",
		Candidates: []assemblyline.DatabaseSchemaCandidate{
			{RelationID: "relation_a", Descriptor: "first relation"},
			{RelationID: "relation_b", Descriptor: "second relation"},
		},
		MaxSelections: 2,
	}
	call, kinds := scriptedObjectiveDatabaseRawLeaves(t, []string{
		assemblyline.DatabaseSchemaRelationRemains,
		"relation_b",
		assemblyline.DatabaseSchemaNoUncoveredRelation,
	})
	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(t.Context(), input, call)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || !reflect.DeepEqual(decision.RelationIDs, []string{"relation_b"}) {
		t.Fatalf("calls=%d decision=%+v", calls, decision)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseSchemaSelectionCoverage,
		assemblyline.WorkDatabaseSchemaRelationSelection,
		assemblyline.WorkDatabaseSchemaSelectionCoverage,
	}
	if !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("kinds=%v want=%v", *kinds, wantKinds)
	}
}

func TestObjectiveDatabaseQueryIntentUsesExactSemanticLeafCallCount(t *testing.T) {
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	projection, err := datasource.ProjectSchemaForIntent(snapshot, []string{snapshot.Relations[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.DatabaseQueryIntentInput{
		EvidenceNeedID: "need-intent", ExactNeed: "How many records exist?",
		SchemaProjection: projection, TemporalAsOf: snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
		MaxRows: 50,
	}
	call, kinds := scriptedObjectiveDatabaseRawLeaves(t, []string{
		"scalar",
		"count_rows",
		assemblyline.DatabaseQueryNoUncoveredItem,
		assemblyline.DatabaseQueryNoUncoveredItem,
		assemblyline.DatabaseQueryNoUncoveredItem,
		assemblyline.DatabaseQueryNoUncoveredItem,
		assemblyline.DatabaseQueryNoUncoveredItem,
	})
	decision, calls, err := resolveObjectiveDatabaseQueryIntent(t.Context(), input, call)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 7 || decision.Shape != datasource.ResultScalar ||
		len(decision.Projections) != 1 || decision.Projections[0].Aggregate != datasource.AggregateCountRows {
		t.Fatalf("calls=%d decision=%+v", calls, decision)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseQueryShape,
		assemblyline.WorkDatabaseQueryProjectionAggregate,
		assemblyline.WorkDatabaseQueryFilterCoverage,
		assemblyline.WorkDatabaseQueryWindowCoverage,
		assemblyline.WorkDatabaseQueryExistenceCoverage,
		assemblyline.WorkDatabaseQueryHavingCoverage,
		assemblyline.WorkDatabaseQueryOrderCoverage,
	}
	if !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("kinds=%v want=%v", *kinds, wantKinds)
	}
}

func TestObjectiveDatabaseSetMembershipExtractsFirstValueBeforeCoverage(t *testing.T) {
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	projection, err := datasource.ProjectSchemaForIntent(snapshot, []string{snapshot.Relations[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	state := assemblyline.NewDatabaseQueryIntentLeafState(assemblyline.DatabaseQueryIntentInput{
		EvidenceNeedID: "need-values", ExactNeed: "Which records have either identifier?",
		SchemaProjection: projection, TemporalAsOf: snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
		MaxRows: 50,
	})
	state.FromRelationID = projection.Relations[0].ID
	state.Shape = datasource.ResultRecords
	fieldID := projection.Relations[0].Columns[0].ID
	state.Projections = []datasource.RelationalProjection{{FieldID: fieldID}}
	leaf := assemblyline.DatabaseQueryFilterLeafInput{
		State: state, AcceptedFilters: []datasource.RelationalPredicate{},
		AcceptedValues: []datasource.IntentLiteral{}, FieldID: fieldID,
		Operator: datasource.FilterIn,
	}
	const identifier = "123e4567-e89b-12d3-a456-426614174000"
	call, kinds := scriptedObjectiveDatabaseRawLeaves(t, []string{
		identifier, assemblyline.DatabaseQueryNoUncoveredValue,
	})
	values, calls, err := resolveDatabaseQueryFilterValues(t.Context(), leaf, call, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantValues := []datasource.IntentLiteral{{Type: datasource.LiteralUUID, Value: identifier}}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseQueryFilterValue,
		assemblyline.WorkDatabaseQueryFilterValueCoverage,
	}
	if calls != 2 || !reflect.DeepEqual(values, wantValues) || !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("calls=%d values=%+v kinds=%v", calls, values, *kinds)
	}
}

func TestObjectiveDatabaseRankingExtractsFirstOrderBeforeCoverage(t *testing.T) {
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	projection, err := datasource.ProjectSchemaForIntent(snapshot, []string{snapshot.Relations[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	state := assemblyline.NewDatabaseQueryIntentLeafState(assemblyline.DatabaseQueryIntentInput{
		EvidenceNeedID: "need-ranking", ExactNeed: "Rank identifiers by record count.",
		SchemaProjection: projection, TemporalAsOf: snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
		MaxRows: 50,
	})
	state.FromRelationID = projection.Relations[0].ID
	state.Shape = datasource.ResultRanking
	state.Projections = []datasource.RelationalProjection{
		{FieldID: projection.Relations[0].Columns[0].ID},
		{Aggregate: datasource.AggregateCountRows},
	}
	call, kinds := scriptedObjectiveDatabaseRawLeaves(t, []string{
		"1", "desc", assemblyline.DatabaseQueryNoUncoveredItem,
	})
	resolved, calls, err := resolveDatabaseQueryOrder(t.Context(), state, call, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []datasource.OrderTerm{{Projection: 1, Direction: datasource.OrderDescending}}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseQueryOrderProjection,
		assemblyline.WorkDatabaseQueryOrderDirection,
		assemblyline.WorkDatabaseQueryOrderCoverage,
	}
	if calls != 3 || !reflect.DeepEqual(resolved.OrderBy, wantOrder) || !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("calls=%d order=%+v kinds=%v", calls, resolved.OrderBy, *kinds)
	}
}

func TestObjectiveDatabaseRawLeafRejectsJSONWithoutSpendingAnotherResponsibility(t *testing.T) {
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	projection, err := datasource.ProjectSchemaForIntent(snapshot, []string{snapshot.Relations[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.DatabaseQueryIntentInput{
		EvidenceNeedID: "need-json", ExactNeed: "How many records exist?",
		SchemaProjection: projection, TemporalAsOf: snapshot.CapturedAt.UTC().Format(time.RFC3339Nano), MaxRows: 50,
	}
	call, kinds := scriptedObjectiveDatabaseRawLeaves(t, []string{`{"shape":"scalar"}`})
	_, calls, err := resolveObjectiveDatabaseQueryIntent(t.Context(), input, call)
	if err == nil {
		t.Fatal("database query intent accepted a JSON model response")
	}
	if calls != 1 || len(*kinds) != 1 || (*kinds)[0] != assemblyline.WorkDatabaseQueryShape {
		t.Fatalf("calls=%d kinds=%v err=%v", calls, *kinds, err)
	}
}

func scriptedObjectiveDatabaseRawLeaves(
	t *testing.T,
	responses []string,
) (objectiveDatabaseRawLeafCall, *[]assemblyline.WorkKind) {
	t.Helper()
	next := 0
	kinds := []assemblyline.WorkKind{}
	call := func(
		_ context.Context,
		_ string,
		job assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		kinds = append(kinds, job.Kind)
		if next >= len(responses) {
			return nil, 0, fmt.Errorf("unexpected database leaf call %s", job.Kind)
		}
		raw := responses[next]
		next++
		value, err := decode(raw)
		return value, 1, err
	}
	return call, &kinds
}
