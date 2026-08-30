package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func TestObjectiveDatabaseSchemaSelectionUsesInventoryAndCodeOwnedCandidateQueue(t *testing.T) {
	const (
		appointments     = "The appointment records containing missed outcomes."
		optional         = "A relation containing display preferences."
		appointmentAlias = "Records of appointments and whether they were missed."
		clinics          = "The clinic records used to identify each clinic."
	)
	input := assemblyline.DatabaseSchemaSelectionInput{
		EvidenceNeedID: "need-schema", ExactNeed: "Which clinics have the most missed appointments?",
		Candidates: []assemblyline.DatabaseSchemaCandidate{
			{RelationID: "relation_a", Descriptor: "clinic identity records"},
			{RelationID: "relation_b", Descriptor: "appointment records with missed status"},
		},
		MaxSelections: 2,
	}
	kinds := []assemblyline.WorkKind{}
	necessityCandidates := map[string]int{}
	resolutionCandidates := map[string]int{}
	call := func(
		_ context.Context,
		_ string,
		job assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		kinds = append(kinds, job.Kind)
		var raw string
		switch job.Kind {
		case assemblyline.WorkDatabaseSchemaRelationInventory:
			raw = appointments + "\n" + optional + "\n" + appointments + "\n" + optional + "\n" +
				appointmentAlias + "\n" + clinics
		case assemblyline.WorkDatabaseSchemaRelationNecessity:
			var leaf assemblyline.DatabaseSchemaRelationNecessityInput
			if err := json.Unmarshal(job.Payload, &leaf); err != nil {
				return nil, 1, err
			}
			necessityCandidates[leaf.Candidate]++
			if leaf.Candidate == optional {
				raw = assemblyline.DatabaseSchemaRelationNotNecessary
			} else {
				raw = assemblyline.DatabaseSchemaRelationNecessary
			}
		case assemblyline.WorkDatabaseSchemaRelationResolution:
			var leaf assemblyline.DatabaseSchemaRelationResolutionInput
			if err := json.Unmarshal(job.Payload, &leaf); err != nil {
				return nil, 1, err
			}
			resolutionCandidates[leaf.Candidate]++
			if leaf.Candidate == clinics {
				raw = "relation_a"
			} else {
				raw = "relation_b"
			}
		default:
			return nil, 1, fmt.Errorf("unexpected schema selection work kind %q", job.Kind)
		}
		value, err := decode(raw)
		return value, 1, err
	}
	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(t.Context(), input, call)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 8 || !reflect.DeepEqual(decision.RelationIDs, []string{"relation_b", "relation_a"}) {
		t.Fatalf("calls=%d decision=%+v", calls, decision)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseSchemaRelationInventory,
		assemblyline.WorkDatabaseSchemaRelationNecessity,
		assemblyline.WorkDatabaseSchemaRelationResolution,
		assemblyline.WorkDatabaseSchemaRelationNecessity,
		assemblyline.WorkDatabaseSchemaRelationNecessity,
		assemblyline.WorkDatabaseSchemaRelationResolution,
		assemblyline.WorkDatabaseSchemaRelationNecessity,
		assemblyline.WorkDatabaseSchemaRelationResolution,
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("kinds=%v want=%v", kinds, wantKinds)
	}
	if necessityCandidates[appointments] != 1 {
		t.Fatalf("byte-identical inventory duplicate reopened candidate: %+v", necessityCandidates)
	}
	if necessityCandidates[optional] != 1 {
		t.Fatalf("byte-identical rejected inventory duplicate reopened candidate: %+v", necessityCandidates)
	}
	if resolutionCandidates[appointmentAlias] != 1 || len(decision.RelationIDs) != 2 {
		t.Fatalf("resolved duplicate relation reopened accepted state: resolutions=%+v decision=%+v", resolutionCandidates, decision)
	}
}

func TestObjectiveDatabaseSchemaSelectionEndsOnQueueExhaustionWithoutResolution(t *testing.T) {
	input := assemblyline.DatabaseSchemaSelectionInput{
		EvidenceNeedID: "need-schema-empty", ExactNeed: "Which clinics have missed appointments?",
		Candidates: []assemblyline.DatabaseSchemaCandidate{
			{RelationID: "relation_a", Descriptor: "unrelated display preferences"},
		},
		MaxSelections: 1,
	}
	call, kinds := scriptedObjectiveDatabaseRawLeaves(t, []string{
		"A relation containing display preferences.",
		assemblyline.DatabaseSchemaRelationNotNecessary,
	})
	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(t.Context(), input, call)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseSchemaRelationInventory,
		assemblyline.WorkDatabaseSchemaRelationNecessity,
	}
	if calls != 2 || len(decision.RelationIDs) != 0 || !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("calls=%d decision=%+v kinds=%v", calls, decision, *kinds)
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
		"count the records",
		assemblyline.DatabaseQueryPurposeNecessary,
		"count_rows",
		assemblyline.DatabaseQueryNoPurposeCandidates,
		assemblyline.DatabaseQueryNoPurposeCandidates,
		assemblyline.DatabaseQueryNoPurposeCandidates,
		assemblyline.DatabaseQueryNoPurposeCandidates,
		assemblyline.DatabaseQueryNoPurposeCandidates,
	})
	decision, calls, err := resolveObjectiveDatabaseQueryIntent(t.Context(), input, call)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 9 || decision.Shape != datasource.ResultScalar ||
		len(decision.Projections) != 1 || decision.Projections[0].Aggregate != datasource.AggregateCountRows {
		t.Fatalf("calls=%d decision=%+v", calls, decision)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseQueryShape,
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryProjectionAggregate,
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeInventory,
	}
	if !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("kinds=%v want=%v", *kinds, wantKinds)
	}
}

func TestObjectiveDatabaseSetMembershipFillsOneAcceptedValuePurpose(t *testing.T) {
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
		Operator: datasource.FilterIn, Purpose: "match either requested identifier",
	}
	const identifier = "123e4567-e89b-12d3-a456-426614174000"
	call, kinds := scriptedObjectiveDatabaseRawLeaves(t, []string{
		"the requested identifier",
		assemblyline.DatabaseQueryPurposeNecessary,
		identifier,
	})
	values, calls, err := resolveDatabaseQueryFilterValues(t.Context(), leaf, call, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantValues := []datasource.IntentLiteral{{Type: datasource.LiteralUUID, Value: identifier}}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryFilterValue,
	}
	if calls != 3 || !reflect.DeepEqual(values, wantValues) || !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("calls=%d values=%+v kinds=%v", calls, values, *kinds)
	}
}

func TestObjectiveDatabaseRankingFillsOneAcceptedOrderPurpose(t *testing.T) {
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
		"sort by record count",
		assemblyline.DatabaseQueryPurposeNecessary,
		"1", "desc",
	})
	resolved, calls, err := resolveDatabaseQueryOrder(t.Context(), state, call, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []datasource.OrderTerm{{Projection: 1, Direction: datasource.OrderDescending}}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryOrderProjection,
		assemblyline.WorkDatabaseQueryOrderDirection,
	}
	if calls != 4 || !reflect.DeepEqual(resolved.OrderBy, wantOrder) || !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("calls=%d order=%+v kinds=%v", calls, resolved.OrderBy, *kinds)
	}
}

func TestObjectiveDatabasePurposeQueueEvaporatesUnrequestedAndDuplicateCandidates(t *testing.T) {
	snapshot := objectiveDatabaseSingleRelationSnapshot(t)
	projection, err := datasource.ProjectSchemaForIntent(snapshot, []string{snapshot.Relations[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	state := assemblyline.NewDatabaseQueryIntentLeafState(assemblyline.DatabaseQueryIntentInput{
		EvidenceNeedID: "need-filter-sieve", ExactNeed: "Return records in the requested state.",
		SchemaProjection: projection, TemporalAsOf: snapshot.CapturedAt.UTC().Format(time.RFC3339Nano),
		MaxRows: 50,
	})
	state.FromRelationID = projection.Relations[0].ID
	state.Shape = datasource.ResultRecords
	authority := assemblyline.DatabaseQueryPurposeAuthority{
		State: state, Collection: assemblyline.DatabaseQueryFilterPurpose,
	}
	call, kinds := scriptedObjectiveDatabaseRawLeaves(t, []string{
		"match the requested state\ninvent a display preference filter\nmatch the requested state\nfilter by the requested state",
		assemblyline.DatabaseQueryPurposeNecessary,
		assemblyline.DatabaseQueryPurposeNotNecessary,
		assemblyline.DatabaseQueryPurposeNecessary,
		assemblyline.DatabaseQueryPurposesSame,
	})
	purposes, calls, err := resolveDatabaseQueryPurposeQueue(t.Context(), authority, 4, call, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantPurposes := []string{"match the requested state"}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryPurposeRelation,
	}
	if calls != 5 || !reflect.DeepEqual(purposes, wantPurposes) || !reflect.DeepEqual(*kinds, wantKinds) {
		t.Fatalf("calls=%d purposes=%v kinds=%v", calls, purposes, *kinds)
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
