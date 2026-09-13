package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseParameterPromptsIgnoreUnrelatedAcceptedClauses(t *testing.T) {
	for _, fixture := range []struct{ relation, value, timestamp string }{
		{"shipments", "weight", "dispatched_at"},
		{"samples", "reading", "observed_at"},
	} {
		t.Run(fixture.relation, func(t *testing.T) {
			base := databaseParameterStateFixture(fixture.relation, fixture.value, fixture.timestamp)
			enriched := withUnrelatedAcceptedQueryClauses(base)
			if err := enriched.validateReady(); err != nil {
				t.Fatalf("unrelated-clause fixture is not valid accepted state: %v", err)
			}
			before, after := databaseParameterCases(t, base), databaseParameterCases(t, enriched)
			for index, original := range before {
				t.Run(string(original.job.Kind), func(t *testing.T) {
					next := after[index]
					first, err := RenderPortableJob(original.job)
					if err != nil {
						t.Fatal(err)
					}
					second, err := RenderPortableJob(next.job)
					if err != nil {
						t.Fatal(err)
					}
					if first != second {
						t.Errorf("unrelated accepted clauses changed the focused parameter prompt\nbefore:\n%s\nafter:\n%s", first, second)
					}
					t.Logf("focused input: %d bytes; unchanged after unrelated accepted work", len(second))
					for _, required := range original.required {
						if !strings.Contains(second, required) {
							t.Errorf("focused parameter omitted required content %q", required)
						}
					}
					for _, unrelated := range []string{
						"ACCEPTED FROM RELATION", "ACCEPTED RESULT SHAPE", "ACCEPTED PROJECTIONS",
						"ACCEPTED FILTERS", "ACCEPTED TEMPORAL WINDOWS", "ACCEPTED EXISTENCE PREDICATES",
						"ACCEPTED HAVING PREDICATES", "ACCEPTED ORDER TERMS", "ACCEPTED VALUES",
						"unrelated_", "unrelated-value", "need-private", "source-private", base.Authority.ExactNeed,
					} {
						if strings.Contains(second, unrelated) {
							t.Errorf("focused parameter exposed unrelated retained state %q", unrelated)
						}
					}
					if err := original.decode(); err != nil {
						t.Errorf("decode original parameter: %v", err)
					}
					if err := next.decode(); err != nil {
						t.Errorf("decode same parameter with unrelated accepted work: %v", err)
					}
				})
			}
			if !reflect.DeepEqual(enriched, withUnrelatedAcceptedQueryClauses(base)) {
				t.Fatal("rendering or decoding changed accepted query state")
			}
		})
	}
}

func TestDatabaseParameterPromptsStillUseChangedFocusedMeaning(t *testing.T) {
	base := databaseParameterStateFixture("readings", "measurement", "observed_at")
	changed := databaseParameterStateFixture("readings", "voltage", "recorded_at")
	before, after := databaseParameterCases(t, base), databaseParameterCases(t, changed)
	for index, original := range before {
		if original.job.Kind == WorkDatabaseQueryExistenceNegated {
			continue
		}
		first, err := RenderPortableJob(original.job)
		if err != nil {
			t.Fatal(err)
		}
		second, err := RenderPortableJob(after[index].job)
		if err != nil {
			t.Fatal(err)
		}
		if first == second {
			t.Errorf("%s ignored changed focused meaning", original.job.Kind)
		}
	}
}

func TestDatabaseClosedParameterShowsOnlyRemainingValues(t *testing.T) {
	state := databaseParameterStateFixture("documents", "state", "created_at")
	column := &state.Authority.SchemaProjection.Relations[0].Columns[0]
	column.TypeCategory = datasource.TypeText
	column.AllowedValues = []string{"draft", "issued", "settled"}
	input := DatabaseQueryFilterLeafInput{
		State: state, Purpose: "The issued state.", ParentPurpose: "Match the requested document states.",
		FieldID: "focus-value", Operator: datasource.FilterIn, AcceptedFilters: []datasource.RelationalPredicate{},
		AcceptedValues: []datasource.IntentLiteral{{Type: datasource.LiteralString, Value: "draft"}},
	}
	job, err := NewDatabaseQueryFilterValueChoiceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{input.Purpose, input.ParentPurpose, "public.documents.state", "issued", "settled"} {
		if !strings.Contains(prompt, required) {
			t.Errorf("closed parameter omitted required meaning %q", required)
		}
	}
	for _, omitted := range []string{"draft", "allowed values:", "ACCEPTED VALUES"} {
		if strings.Contains(prompt, omitted) {
			t.Errorf("closed parameter exposed retained or duplicated values %q", omitted)
		}
	}
	value, err := DecodeDatabaseQueryFilterValueChoice(input, "A")
	if err != nil || value.Value == nil || *value.Value != (datasource.IntentLiteral{Type: datasource.LiteralString, Value: "issued"}) {
		t.Fatalf("remaining choice did not bind to its actual value: %v %v", value, err)
	}
	if !reflect.DeepEqual(column.AllowedValues, []string{"draft", "issued", "settled"}) ||
		len(input.AcceptedValues) != 1 || input.AcceptedValues[0].Value != "draft" {
		t.Fatal("rendering or decoding changed retained values")
	}
}

func TestDatabaseFocusedPromptStillRejectsInvalidAcceptedState(t *testing.T) {
	state := databaseParameterStateFixture("measurements", "value", "created_at")
	state.Filters = []datasource.RelationalPredicate{{FieldID: "missing", Operator: datasource.FilterIsNull}}
	for _, current := range databaseParameterCases(t, state) {
		if _, err := RenderPortableJob(current.job); err == nil {
			t.Errorf("%s ignored an invalid retained field reference", current.job.Kind)
		}
		if err := current.decode(); err == nil {
			t.Errorf("%s accepted a result against invalid retained state", current.job.Kind)
		}
	}
}
