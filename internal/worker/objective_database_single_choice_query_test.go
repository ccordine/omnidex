package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func TestQueryIntentUsesSoleFromRelationWithoutFromRelationCall(t *testing.T) {
	input := databaseSingleChoiceIntentInput()
	input.ExactNeed = "Return each metric name."
	input.SchemaProjection.Relations[0].Columns = []datasource.IntentColumnProjection{
		{ID: "name", Name: "name", TypeCategory: datasource.TypeText},
	}
	fromRelationCalls := 0
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
		case "database_query_from_relation":
			fromRelationCalls++
			return nil, 1, fmt.Errorf("sole from relation reached the model call")
		case "database_query_shape":
			raw = "A"
		case "database_query_purpose_presence":
			raw = "B"
		case "database_query_purpose_inventory":
			raw = "Return the metric name"
		case "database_query_purpose_necessity":
			raw = "A"
		case "database_query_projection_field":
			return nil, 1, fmt.Errorf("sole projection field reached the model call")
		default:
			return nil, 1, fmt.Errorf("unexpected database leaf %q", subject)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	decision, calls, err := resolveObjectiveDatabaseQueryIntent(
		context.Background(), input, call,
	)
	if err != nil {
		t.Fatalf("resolve query intent: %v", err)
	}
	if calls != 8 || providerCalls != 8 {
		t.Fatalf("provider calls = reported %d actual %d, want 8 non-from calls", calls, providerCalls)
	}
	if fromRelationCalls != 0 {
		t.Fatalf("sole from relation made %d provider calls", fromRelationCalls)
	}
	if decision.FromRelationID != "metrics" {
		t.Fatalf("from relation = %q, want metrics", decision.FromRelationID)
	}
}

func TestWindowUsesSoleTemporalFieldWithoutFieldCall(t *testing.T) {
	state := databaseSingleChoiceStateWithColumns([]datasource.IntentColumnProjection{
		{ID: "created-date", Name: "created_date", TypeCategory: datasource.TypeDate},
	})
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
			raw = "Only records from the previous day"
		case "database_query_purpose_necessity":
			raw = "A"
		case "database_query_window_field":
			fieldCalls++
			return nil, 1, fmt.Errorf("sole temporal field reached the model call")
		case "database_query_window_unit":
			raw = "A"
		case "database_query_window_amount":
			raw = "1"
		default:
			return nil, 1, fmt.Errorf("unexpected database leaf %q", subject)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	result, calls, err := resolveDatabaseQueryWindows(context.Background(), state, call, 0)
	if err != nil {
		t.Fatalf("resolve temporal windows: %v", err)
	}
	if calls != 5 || providerCalls != 5 {
		t.Fatalf("provider calls = reported %d actual %d, want 5 non-field calls", calls, providerCalls)
	}
	if fieldCalls != 0 {
		t.Fatalf("sole temporal field made %d provider calls", fieldCalls)
	}
	if len(result.TemporalWindows) != 1 || result.TemporalWindows[0].FieldID != "created-date" {
		t.Fatalf("temporal windows = %#v, want created-date", result.TemporalWindows)
	}
}

func TestExistenceUsesSoleRelationWithoutRelationCall(t *testing.T) {
	state := databaseSingleChoiceStateWithColumns([]datasource.IntentColumnProjection{
		{ID: "name", Name: "name", TypeCategory: datasource.TypeText},
	})
	relationCalls := 0
	presenceCalls := 0
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
			presenceCalls++
			if presenceCalls == 1 {
				raw = "A"
			} else {
				raw = "B"
			}
		case "database_query_purpose_inventory":
			raw = "Require a matching metric record"
		case "database_query_purpose_necessity":
			raw = "A"
		case "database_query_existence_relation":
			relationCalls++
			return nil, 1, fmt.Errorf("sole existence relation reached the model call")
		case "database_query_existence_negated":
			raw = "A"
		default:
			return nil, 1, fmt.Errorf("unexpected database leaf %q", subject)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	result, calls, err := resolveDatabaseQueryExistence(context.Background(), state, call, 0)
	if err != nil {
		t.Fatalf("resolve existence: %v", err)
	}
	if calls != 5 || providerCalls != 5 {
		t.Fatalf("provider calls = reported %d actual %d, want 5 non-relation calls", calls, providerCalls)
	}
	if relationCalls != 0 {
		t.Fatalf("sole existence relation made %d provider calls", relationCalls)
	}
	if len(result.Exists) != 1 || result.Exists[0].RelationID != "metrics" {
		t.Fatalf("existence predicates = %#v, want metrics", result.Exists)
	}
}

func databaseSingleChoiceStateWithColumns(
	columns []datasource.IntentColumnProjection,
) assemblyline.DatabaseQueryIntentLeafState {
	input := databaseSingleChoiceIntentInput()
	input.SchemaProjection.Relations[0].Columns = columns
	state := assemblyline.NewDatabaseQueryIntentLeafState(input)
	state.FromRelationID = "metrics"
	state.Shape = datasource.ResultRecords
	return state
}
