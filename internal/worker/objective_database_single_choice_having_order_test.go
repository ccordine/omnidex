package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func TestHavingUsesCanonicalSoleFieldWithoutFieldCall(t *testing.T) {
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
			raw = "Require a positive sum"
		case "database_query_purpose_necessity":
			raw = "A"
		case "database_query_having_aggregate":
			raw = "D"
		case "database_query_having_field":
			fieldCalls++
			return nil, 1, fmt.Errorf("canonical sole having field reached the model call")
		case "database_query_having_operator":
			raw = "C"
		case "database_query_having_value":
			raw = "0"
		default:
			return nil, 1, fmt.Errorf("unexpected database leaf %q", subject)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	result, calls, err := resolveDatabaseQueryHaving(context.Background(), state, call, 0)
	if err != nil {
		t.Fatalf("resolve having: %v", err)
	}
	if calls != 6 || providerCalls != 6 {
		t.Fatalf("provider calls = reported %d actual %d, want 6 non-field calls", calls, providerCalls)
	}
	if fieldCalls != 0 {
		t.Fatalf("canonical sole having field made %d provider calls", fieldCalls)
	}
	if len(result.Having) != 1 || result.Having[0].FieldID != "numeric-value" {
		t.Fatalf("having predicates = %#v, want numeric-value", result.Having)
	}
}

func TestOrderUsesSoleProjectionWithoutProjectionCall(t *testing.T) {
	state := databaseSingleChoiceStateWithColumns([]datasource.IntentColumnProjection{
		{ID: "numeric-value", Name: "numeric_value", TypeCategory: datasource.TypeInteger},
	})
	state.Projections = []datasource.RelationalProjection{{FieldID: "numeric-value"}}
	projectionCalls := 0
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
			raw = "Order by the numeric measurement"
		case "database_query_purpose_necessity":
			raw = "A"
		case "database_query_order_projection":
			projectionCalls++
			return nil, 1, fmt.Errorf("sole order projection reached the model call")
		case "database_query_order_direction":
			raw = "A"
		default:
			return nil, 1, fmt.Errorf("unexpected database leaf %q", subject)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	result, calls, err := resolveDatabaseQueryOrder(context.Background(), state, call, 0)
	if err != nil {
		t.Fatalf("resolve order: %v", err)
	}
	if calls != 4 || providerCalls != 4 {
		t.Fatalf("provider calls = reported %d actual %d, want 4 non-projection calls", calls, providerCalls)
	}
	if projectionCalls != 0 {
		t.Fatalf("sole order projection made %d provider calls", projectionCalls)
	}
	if len(result.OrderBy) != 1 || result.OrderBy[0].Projection != 0 {
		t.Fatalf("order terms = %#v, want projection zero", result.OrderBy)
	}
}
