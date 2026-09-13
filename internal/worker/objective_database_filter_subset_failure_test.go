package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseFilterSubsetFailuresNeverOpenAnotherStation(t *testing.T) {
	for _, fixture := range []struct {
		name, raw, want string
	}{
		{"empty subset", "C", "no applicable value"},
		{"unknown choice", "Z", "unavailable"},
		{"list response", "A, B", "exceeds"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			leaf := databaseFilterSubsetInput(datasource.IntentColumnProjection{
				ID: "mode", Name: "mode", TypeCategory: datasource.TypeText, AllowedValues: []string{"active", "idle"},
			}, "Include the requested modes.")
			providerCalls := 0
			call := func(_ context.Context, _ string, job assemblyline.PortableJob, decode objectiveDatabaseRawLeafDecoder) (any, int, error) {
				providerCalls++
				if job.Kind != assemblyline.WorkDatabaseQueryFilterValueChoice {
					t.Fatalf("failure opened %s", job.Kind)
				}
				value, err := decode(fixture.raw)
				return value, 1, err
			}
			values, calls, err := resolveDatabaseQueryFilterValues(context.Background(), leaf, call, 0)
			if values != nil || err == nil || !strings.Contains(err.Error(), fixture.want) || calls != 1 || providerCalls != 1 {
				t.Fatalf("values=%v calls=%d provider=%d error=%v", values, calls, providerCalls, err)
			}
			if len(leaf.AcceptedValues) != 0 {
				t.Fatal("failed selection changed caller-owned state")
			}
		})
	}
}

func TestDatabaseFilterSubsetCompletesExhaustionWithoutAnExtraCall(t *testing.T) {
	leaf := databaseFilterSubsetInput(datasource.IntentColumnProjection{
		ID: "enabled", Name: "enabled", TypeCategory: datasource.TypeBoolean,
	}, "Exclude either boolean value.")
	leaf.Operator = datasource.FilterNotIn
	providerCalls := 0
	values, calls, err := resolveDatabaseQueryFilterValues(context.Background(), leaf,
		func(_ context.Context, _ string, _ assemblyline.PortableJob, decode objectiveDatabaseRawLeafDecoder) (any, int, error) {
			providerCalls++
			value, err := decode("A")
			return value, 1, err
		}, 0)
	if err != nil || len(values) != 2 || calls != 2 || providerCalls != 2 {
		t.Fatalf("values=%v calls=%d provider=%d error=%v", values, calls, providerCalls, err)
	}
}

func TestDatabaseFilterSubsetDoesNotSilentlyTruncateAtPredicateLimit(t *testing.T) {
	for _, lastResponse := range []string{"A", "B"} {
		t.Run(lastResponse, func(t *testing.T) {
			allowed := make([]string, datasource.MaxIntentFilterValues+1)
			for index := range allowed {
				allowed[index] = fmt.Sprintf("state-%d", index)
			}
			leaf := databaseFilterSubsetInput(datasource.IntentColumnProjection{
				ID: "state", Name: "state", TypeCategory: datasource.TypeText, AllowedValues: allowed,
			}, "Match the requested states.")
			providerCalls := 0
			values, calls, err := resolveDatabaseQueryFilterValues(context.Background(), leaf,
				func(_ context.Context, _ string, _ assemblyline.PortableJob, decode objectiveDatabaseRawLeafDecoder) (any, int, error) {
					providerCalls++
					raw := "A"
					if providerCalls == len(allowed) {
						raw = lastResponse
					}
					value, err := decode(raw)
					return value, 1, err
				}, 0)
			if calls != len(allowed) || providerCalls != calls {
				t.Fatalf("calls=%d provider=%d", calls, providerCalls)
			}
			if lastResponse == "A" {
				if err == nil || values != nil || !strings.Contains(err.Error(), "requires 1..50 values") {
					t.Fatalf("oversized selected set was not rejected: %v / %v", values, err)
				}
			} else if err != nil || len(values) != datasource.MaxIntentFilterValues {
				t.Fatalf("valid set at the limit failed: %v / %v", values, err)
			}
		})
	}
}
