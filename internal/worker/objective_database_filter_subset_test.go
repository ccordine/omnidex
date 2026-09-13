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

func TestDatabaseClosedFilterValuesUseRemainingChoiceRounds(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		column  datasource.IntentColumnProjection
		purpose string
		raw     []string
		want    []string
	}{
		{
			name: "enum membership",
			column: datasource.IntentColumnProjection{
				ID: "state", Name: "state", TypeCategory: datasource.TypeText,
				AllowedValues: []string{"draft", "published", "archived"},
			},
			purpose: "Include drafts and published records.",
			raw:     []string{"A", "A", "B"}, want: []string{"draft", "published"},
		},
		{
			name: "boolean membership",
			column: datasource.IntentColumnProjection{
				ID: "enabled", Name: "enabled", TypeCategory: datasource.TypeBoolean,
			},
			purpose: "Include enabled records.",
			raw:     []string{"A", "B"}, want: []string{"true"},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			leaf := databaseFilterSubsetInput(fixture.column, fixture.purpose)
			calls := 0
			call := func(_ context.Context, _ string, job assemblyline.PortableJob, decode objectiveDatabaseRawLeafDecoder) (any, int, error) {
				if job.Kind != assemblyline.WorkDatabaseQueryFilterValueChoice {
					return nil, 1, fmt.Errorf("known value set reached unrelated station %q", job.Kind)
				}
				if calls >= len(fixture.raw) {
					return nil, 1, fmt.Errorf("value selection continued after the absence result")
				}
				prompt, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return nil, 0, err
				}
				for _, selected := range fixture.want[:min(calls, len(fixture.want))] {
					if strings.Contains(prompt, "The exact value \""+selected+"\"") {
						t.Fatalf("accepted value %q was offered again: %s", selected, prompt)
					}
				}
				value, err := decode(fixture.raw[calls])
				calls++
				return value, 1, err
			}
			values, total, err := resolveDatabaseQueryFilterValues(context.Background(), leaf, call, 7)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(values))
			for index, value := range values {
				got[index] = value.Value
			}
			if !reflect.DeepEqual(got, fixture.want) || calls != len(fixture.raw) || total != 7+calls {
				t.Fatalf("values=%v calls=%d total=%d", got, calls, total)
			}
		})
	}
}

func TestDatabaseClosedFilterSoleValueNeedsNoStation(t *testing.T) {
	leaf := databaseFilterSubsetInput(datasource.IntentColumnProjection{
		ID: "state", Name: "state", TypeCategory: datasource.TypeText,
		AllowedValues: []string{"ready"},
	}, "Include ready records.")
	values, calls, err := resolveDatabaseQueryFilterValues(context.Background(), leaf, nil, 3)
	if err != nil || len(values) != 1 || values[0].Value != "ready" || calls != 3 {
		t.Fatalf("values=%v calls=%d err=%v", values, calls, err)
	}
}

func TestDatabaseOpenFilterValuesStillResolveUnknownLiteralMeanings(t *testing.T) {
	leaf := databaseFilterSubsetInput(datasource.IntentColumnProjection{
		ID: "name", Name: "name", TypeCategory: datasource.TypeText,
	}, "Match Ada or Lin.")
	raw := []string{"Match Ada.\nMatch Lin.", "A", "A", "B", "Ada", "Lin"}
	kinds := []assemblyline.WorkKind{
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeNecessity, assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryPurposeRelation,
		assemblyline.WorkDatabaseQueryFilterValue, assemblyline.WorkDatabaseQueryFilterValue,
	}
	providerCalls := 0
	values, calls, err := resolveDatabaseQueryFilterValues(context.Background(), leaf,
		func(_ context.Context, _ string, job assemblyline.PortableJob, decode objectiveDatabaseRawLeafDecoder) (any, int, error) {
			if providerCalls >= len(kinds) || job.Kind != kinds[providerCalls] {
				return nil, 1, fmt.Errorf("unexpected open-domain call %s at %d", job.Kind, providerCalls)
			}
			value, err := decode(raw[providerCalls])
			providerCalls++
			return value, 1, err
		}, 0)
	if err != nil || len(values) != 2 || values[0].Value != "Ada" || values[1].Value != "Lin" || calls != len(raw) || calls != providerCalls {
		t.Fatalf("values=%v calls=%d provider=%d error=%v", values, calls, providerCalls, err)
	}
}

func databaseFilterSubsetInput(column datasource.IntentColumnProjection, purpose string) assemblyline.DatabaseQueryFilterLeafInput {
	return assemblyline.DatabaseQueryFilterLeafInput{
		State:   databaseSingleChoiceStateWithColumns([]datasource.IntentColumnProjection{column}),
		Purpose: purpose, FieldID: column.ID, Operator: datasource.FilterIn,
		AcceptedFilters: []datasource.RelationalPredicate{}, AcceptedValues: []datasource.IntentLiteral{},
	}
}
