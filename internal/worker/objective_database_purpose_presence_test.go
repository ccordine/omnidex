package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func TestOptionalDatabasePurposeAbsenceSkipsPositiveInventory(t *testing.T) {
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
		if subject != "database_query_purpose_presence" {
			return nil, 1, fmt.Errorf("absence opened unexpected leaf %q", subject)
		}
		value, err := decode("B")
		return value, 1, err
	}

	purposes, calls, err := resolveDatabaseQueryPurposeQueue(
		context.Background(),
		assemblyline.DatabaseQueryPurposeAuthority{
			State: state, Collection: assemblyline.DatabaseQueryFilterPurpose,
		},
		4, false, call, 0,
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

func TestRequiredDatabasePurposeOpensPositiveInventoryWithoutPresenceCall(t *testing.T) {
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
		case "database_query_purpose_presence":
			return nil, 1, fmt.Errorf("required collection opened a presence call")
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
		1, true, call, 0,
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
