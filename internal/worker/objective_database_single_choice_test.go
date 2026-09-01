package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func TestSelectDatabaseRelationsUsesSoleSnapshotRelationWithoutStation(t *testing.T) {
	snapshot, err := datasource.NewSchemaSnapshot(
		"source-1",
		"fixture",
		[]datasource.RelationDefinition{
			{
				Schema: "public", Name: "customers", Kind: datasource.RelationTable,
				Columns: []datasource.ColumnDefinition{
					{
						Name: "id", Ordinal: 1, DataType: "bigint",
						TypeCategory: datasource.TypeInteger,
					},
				},
			},
		},
		time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("construct sole-relation snapshot: %v", err)
	}

	selected, receipt, err := selectObjectiveDatabaseRelations(
		context.Background(),
		snapshot,
		"need-1",
		"Read customer records.",
		assemblyline.ObjectiveContext{},
		nil,
	)
	if err != nil {
		t.Fatalf("select sole snapshot relation: %v", err)
	}
	if len(selected) != 1 || selected[0] != snapshot.Relations[0].ID {
		t.Fatalf("selected relations = %#v, want sole relation %q", selected, snapshot.Relations[0].ID)
	}
	if receipt != (objectiveStationReceipt{}) {
		t.Fatalf("sole snapshot relation receipt = %+v, want zero model calls", receipt)
	}
}

func TestSelectDatabaseRelationsKeepsEmptySnapshotFailureBeforeStation(t *testing.T) {
	_, receipt, err := selectObjectiveDatabaseRelations(
		context.Background(),
		datasource.SchemaSnapshot{},
		"need-1",
		"Read customer records.",
		assemblyline.ObjectiveContext{},
		nil,
	)
	if err == nil || err.Error() != "database schema snapshot has no relations" {
		t.Fatalf("empty snapshot error = %v", err)
	}
	if receipt != (objectiveStationReceipt{}) {
		t.Fatalf("empty snapshot receipt = %+v, want zero model calls", receipt)
	}
}

func TestSelectJoinPathUsesSoleOptionWithoutModelDispatch(t *testing.T) {
	input := assemblyline.DatabaseJoinPathSelectionInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Read the orders belonging to each customer.",
		Context:        assemblyline.ObjectiveContext{},
		FromRelationID: "customers",
		ToRelationID:   "orders",
		Candidates: []assemblyline.DatabaseJoinPathCandidate{
			{
				PathID:     "customer-orders",
				Descriptor: "Customer records relate directly to order records through the customer identifier.",
			},
		},
	}

	decision, receipt, err := (portableObjectiveDatabaseStations{}).SelectJoinPath(
		context.Background(), input,
	)
	if err != nil {
		t.Fatalf("select sole join path: %v", err)
	}
	if decision.PathID != "customer-orders" {
		t.Fatalf("selected path = %q, want customer-orders", decision.PathID)
	}
	if receipt != (objectiveStationReceipt{}) {
		t.Fatalf("sole join path receipt = %+v, want zero provider calls", receipt)
	}
}

func TestSchemaSelectionUsesSoleRelationWithoutInference(t *testing.T) {
	input := assemblyline.DatabaseSchemaSelectionInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Read the customer account name.",
		Context:        assemblyline.ObjectiveContext{},
		Candidates: []assemblyline.DatabaseSchemaCandidate{
			{
				RelationID: "customers",
				Descriptor: "Customer account records containing each account name.",
			},
		},
		MaxSelections: 1,
	}
	providerCalls := 0
	call := func(
		_ context.Context,
		subject string,
		_ assemblyline.PortableJob,
		_ objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		providerCalls++
		return nil, 1, fmt.Errorf("sole schema relation reached unexpected leaf %q", subject)
	}

	decision, calls, err := resolveObjectiveDatabaseSchemaSelection(
		context.Background(), input, call,
	)
	if err != nil {
		t.Fatalf("resolve schema selection: %v", err)
	}
	if calls != 0 || providerCalls != 0 {
		t.Fatalf("provider calls = reported %d actual %d, want 0", calls, providerCalls)
	}
	if len(decision.RelationIDs) != 1 || decision.RelationIDs[0] != "customers" {
		t.Fatalf("selected relations = %#v, want customers", decision.RelationIDs)
	}
}

func TestProjectionFieldUsesCanonicalSoleOptionWithoutFieldCall(t *testing.T) {
	state := assemblyline.NewDatabaseQueryIntentLeafState(databaseSingleChoiceIntentInput())
	state.FromRelationID = "metrics"
	state.Shape = datasource.ResultScalar
	fieldCalls := 0
	providerCalls := 0
	call := func(
		_ context.Context,
		subject string,
		_ assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		providerCalls++
		var raw string
		switch subject {
		case "database_query_purpose_presence":
			raw = "A"
		case "database_query_purpose_inventory":
			raw = "The minimum numeric measurement"
		case "database_query_purpose_necessity":
			raw = "A"
		case "database_query_projection_aggregate":
			raw = "F"
		case "database_query_projection_field":
			fieldCalls++
			return nil, 1, fmt.Errorf("canonical sole field reached the model call")
		default:
			return nil, 1, fmt.Errorf("unexpected database leaf %q", subject)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	result, calls, err := resolveDatabaseQueryProjections(
		context.Background(), state, call, 0,
	)
	if err != nil {
		t.Fatalf("resolve projections: %v", err)
	}
	if calls != 3 || providerCalls != 3 {
		t.Fatalf("provider calls = reported %d actual %d, want 3 non-field calls", calls, providerCalls)
	}
	if fieldCalls != 0 {
		t.Fatalf("canonical sole projection field made %d provider calls", fieldCalls)
	}
	if len(result.Projections) != 1 ||
		result.Projections[0].Aggregate != datasource.AggregateMinimum ||
		result.Projections[0].FieldID != "numeric-value" {
		t.Fatalf("projection = %#v, want minimum of numeric-value", result.Projections)
	}
}

func TestClosedFilterFieldAndValueUseSoleOptionsWithoutTheirCalls(t *testing.T) {
	input := databaseSingleChoiceIntentInput()
	input.ExactNeed = "Return enabled metrics."
	input.SchemaProjection.Relations[0].Columns = []datasource.IntentColumnProjection{
		{
			ID:            "status",
			Name:          "status",
			TypeCategory:  datasource.TypeText,
			AllowedValues: []string{"enabled"},
		},
	}
	state := assemblyline.NewDatabaseQueryIntentLeafState(input)
	state.FromRelationID = "metrics"
	state.Shape = datasource.ResultRecords
	fieldCalls := 0
	valueCalls := 0
	providerCalls := 0
	call := func(
		_ context.Context,
		subject string,
		_ assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		providerCalls++
		var raw string
		switch subject {
		case "database_query_purpose_presence":
			raw = "A"
		case "database_query_purpose_inventory":
			raw = "Only enabled metrics"
		case "database_query_purpose_necessity":
			raw = "A"
		case "database_query_filter_field":
			fieldCalls++
			return nil, 1, fmt.Errorf("sole filter field reached the model call")
		case "database_query_filter_operator":
			raw = "A"
		case "database_query_filter_value":
			valueCalls++
			return nil, 1, fmt.Errorf("sole closed filter value reached the model call")
		default:
			return nil, 1, fmt.Errorf("unexpected database leaf %q", subject)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	filters, calls, err := resolveDatabaseQueryFilters(
		context.Background(), state, "", "", []datasource.RelationalPredicate{}, call, 0,
	)
	if err != nil {
		t.Fatalf("resolve filters: %v", err)
	}
	if calls != 4 || providerCalls != 4 {
		t.Fatalf("provider calls = reported %d actual %d, want 4 non-field/value calls", calls, providerCalls)
	}
	if fieldCalls != 0 {
		t.Fatalf("sole filter field made %d provider calls", fieldCalls)
	}
	if valueCalls != 0 {
		t.Fatalf("sole closed filter value made %d provider calls", valueCalls)
	}
	if len(filters) != 1 || len(filters[0].Values) != 1 || filters[0].Values[0].Value != "enabled" {
		t.Fatalf("filters = %#v, want one enabled-value predicate", filters)
	}
}

func databaseSingleChoiceIntentInput() assemblyline.DatabaseQueryIntentInput {
	return assemblyline.DatabaseQueryIntentInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Return the minimum numeric measurement.",
		Context:        assemblyline.ObjectiveContext{},
		SchemaProjection: datasource.IntentSchemaProjection{
			Schema:            datasource.IntentSchemaProjectionV1,
			SourceID:          "source-1",
			SchemaFingerprint: strings.Repeat("a", 64),
			Relations: []datasource.IntentRelationProjection{
				{
					ID:         "metrics",
					SchemaName: "public",
					Name:       "metrics",
					Kind:       datasource.RelationTable,
					Columns: []datasource.IntentColumnProjection{
						{ID: "enabled", Name: "enabled", TypeCategory: datasource.TypeBoolean},
						{ID: "numeric-value", Name: "numeric_value", TypeCategory: datasource.TypeInteger},
					},
				},
			},
		},
		TemporalAsOf: "2026-08-31T12:00:00Z",
		MaxRows:      50,
	}
}
