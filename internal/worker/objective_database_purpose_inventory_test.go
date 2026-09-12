package worker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabasePurposeEmptyInventorySkipsCandidateCalls(t *testing.T) {
	state := databaseSingleChoiceStateWithColumns([]datasource.IntentColumnProjection{
		{ID: "name", Name: "name", TypeCategory: datasource.TypeText},
	})
	providerCalls := 0
	call := func(
		_ context.Context,
		subject string,
		_ assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		providerCalls++
		if subject != "database_query_purpose_inventory" {
			return nil, 1, fmt.Errorf("absence opened unexpected leaf %q", subject)
		}
		value, err := decode("NO_QUERY_PURPOSE_CANDIDATES")
		return value, 1, err
	}

	purposes, calls, err := resolveDatabaseQueryPurposeQueue(
		context.Background(),
		assemblyline.DatabaseQueryPurposeAuthority{
			State: state, Collection: assemblyline.DatabaseQueryFilterPurpose,
		},
		4, call, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(purposes) != 0 || calls != 1 || providerCalls != 1 {
		t.Fatalf(
			"purposes=%#v calls=%d provider_calls=%d, want empty/1/1",
			purposes, calls, providerCalls,
		)
	}
}

func TestDatabasePurposeInventoryDirectlyEntersTheOrdinarySieve(t *testing.T) {
	state := databaseSingleChoiceStateWithColumns([]datasource.IntentColumnProjection{
		{ID: "name", Name: "name", TypeCategory: datasource.TypeText},
	})
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
		case "database_query_purpose_inventory":
			raw = "Return the requested name"
		case "database_query_purpose_necessity":
			raw = "A"
		default:
			return nil, 1, fmt.Errorf("unexpected leaf %q", subject)
		}
		value, err := decode(raw)
		return value, 1, err
	}

	purposes, calls, err := resolveDatabaseQueryPurposeQueue(
		context.Background(),
		assemblyline.DatabaseQueryPurposeAuthority{
			State: state, Collection: assemblyline.DatabaseQueryProjectionPurpose,
		},
		1, call, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(purposes) != 1 || purposes[0] != "Return the requested name" ||
		calls != 2 || providerCalls != 2 {
		t.Fatalf(
			"purposes=%#v calls=%d provider_calls=%d, want one purpose/2/2",
			purposes, calls, providerCalls,
		)
	}
}

func TestDatabasePurposeEmptyRequiredResultsFailAtTheirConsumers(t *testing.T) {
	for _, consumer := range []string{"projection", "ranking", "membership"} {
		t.Run(consumer, func(t *testing.T) {
			state := databaseSingleChoiceStateWithColumns([]datasource.IntentColumnProjection{{ID: "name", Name: "name", TypeCategory: datasource.TypeText}})
			providerCalls := 0
			call := func(_ context.Context, subject string, _ assemblyline.PortableJob, decode objectiveDatabaseRawLeafDecoder) (any, int, error) {
				providerCalls++
				if subject != "database_query_purpose_inventory" {
					t.Errorf("empty inventory opened unexpected leaf %q", subject)
				}
				value, err := decode(assemblyline.DatabaseNoQueryPurposeCandidates)
				return value, 1, err
			}
			var calls int
			var err error
			var wantError string
			switch consumer {
			case "projection":
				_, calls, err = resolveDatabaseQueryProjections(context.Background(), state, call, 0)
				wantError = "before satisfying the code-owned"
			case "ranking":
				state.Shape = datasource.ResultRanking
				state.Projections = []datasource.RelationalProjection{{FieldID: "name"}}
				_, calls, err = resolveDatabaseQueryOrder(context.Background(), state, call, 0)
				wantError = "requires one accepted ordering purpose"
			case "membership":
				_, calls, err = resolveDatabaseQueryFilterValues(context.Background(), assemblyline.DatabaseQueryFilterLeafInput{
					State: state, Purpose: "Match one of the requested names.", FieldID: "name", Operator: datasource.FilterIn,
				}, call, 0)
				wantError = "requires at least one literal purpose"
			}
			if err == nil || !strings.Contains(err.Error(), wantError) || calls != 1 || providerCalls != 1 {
				t.Fatalf("calls=%d provider=%d error=%v; want the consumer's explicit empty-result failure", calls, providerCalls, err)
			}
		})
	}
}

func TestDatabasePurposeZeroCapacityNeedsNoInference(t *testing.T) {
	state := databaseSingleChoiceStateWithColumns([]datasource.IntentColumnProjection{{ID: "name", Name: "name", TypeCategory: datasource.TypeText}})
	purposes, calls, err := resolveDatabaseQueryPurposeQueue(context.Background(), assemblyline.DatabaseQueryPurposeAuthority{
		State: state, Collection: assemblyline.DatabaseQueryFilterPurpose,
	}, 0, nil, 5)
	if err != nil || calls != 5 || !reflect.DeepEqual(purposes, []string{}) {
		t.Fatalf("exhausted capacity purposes=%q calls=%d error=%v", purposes, calls, err)
	}
}
