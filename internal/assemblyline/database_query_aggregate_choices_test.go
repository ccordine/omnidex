package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseAggregateChoicesRequireAnEligibleField(t *testing.T) {
	for _, fixture := range []struct{ relation, first, second string }{
		{"subscribers", "active", "verified"},
		{"circuits", "enabled", "connected"},
	} {
		for _, category := range []datasource.ColumnTypeCategory{
			datasource.TypeBoolean, datasource.TypeText, datasource.TypeDate,
			datasource.TypeInteger, datasource.TypeDecimal,
		} {
			t.Run(fixture.relation+"/"+string(category), func(t *testing.T) {
				state := databaseParameterStateFixture(fixture.relation, fixture.first, fixture.second)
				state.Authority.SchemaProjection.Relations = state.Authority.SchemaProjection.Relations[:1]
				state.Authority.SchemaProjection.Relations[0].Columns = []datasource.IntentColumnProjection{
					{ID: "focus-value", Name: fixture.first, TypeCategory: category},
					{ID: "second", Name: fixture.second, TypeCategory: category},
				}
				projection := []datasource.AggregateOperation{"", datasource.AggregateCountRows, datasource.AggregateCount, datasource.AggregateCountDistinct}
				having := []datasource.AggregateOperation{datasource.AggregateCountRows, datasource.AggregateCount, datasource.AggregateCountDistinct}
				if category == datasource.TypeInteger || category == datasource.TypeDecimal {
					projection = append(projection, datasource.AggregateSum, datasource.AggregateAverage)
					having = append(having, datasource.AggregateSum, datasource.AggregateAverage)
				}
				if category != datasource.TypeBoolean {
					projection = append(projection, datasource.AggregateMinimum, datasource.AggregateMaximum)
				}
				assertDatabaseAggregateChoices(t, state, projection, having)
				state.Shape = datasource.ResultScalar
				assertDatabaseAggregateChoices(t, state, projection[1:], having)
			})
		}
	}
}

func TestDatabaseAggregateChoicesRespectClosedFieldDomains(t *testing.T) {
	state := databaseParameterStateFixture("documents", "state", "created_at")
	state.Authority.SchemaProjection.Relations = state.Authority.SchemaProjection.Relations[:1]
	state.Authority.SchemaProjection.Relations[0].Columns = []datasource.IntentColumnProjection{{
		ID: "focus-value", Name: "state", TypeCategory: datasource.TypeText, AllowedValues: []string{"draft", "issued"},
	}}
	counts := []datasource.AggregateOperation{datasource.AggregateCountRows, datasource.AggregateCount, datasource.AggregateCountDistinct}
	assertDatabaseAggregateChoices(t, state, append([]datasource.AggregateOperation{""}, counts...), counts)
	// Eligibility is determined from every projected field, not just the anchor.
	state.Authority.SchemaProjection.Relations = append(state.Authority.SchemaProjection.Relations,
		datasource.IntentRelationProjection{
			ID: "related", SchemaName: "public", Name: "amounts", Kind: datasource.RelationTable,
			Columns: []datasource.IntentColumnProjection{{ID: "amount", Name: "value", TypeCategory: datasource.TypeDecimal}},
		})
	measures := append(append([]datasource.AggregateOperation{}, counts...), datasource.AggregateSum, datasource.AggregateAverage)
	projection := append([]datasource.AggregateOperation{""}, measures...)
	projection = append(projection, datasource.AggregateMinimum, datasource.AggregateMaximum)
	assertDatabaseAggregateChoices(t, state, projection, measures)
}

func assertDatabaseAggregateChoices(
	t *testing.T, state DatabaseQueryIntentLeafState, projection, having []datasource.AggregateOperation,
) {
	t.Helper()
	projectionInput := DatabaseQueryProjectionLeafInput{State: state, Purpose: "Measure the requested values."}
	projectionJob, err := NewDatabaseQueryProjectionAggregateJob(projectionInput)
	if err != nil {
		t.Fatal(err)
	}
	havingInput := DatabaseQueryHavingLeafInput{State: state, Purpose: "Constrain the requested measure."}
	havingJob, err := NewDatabaseQueryHavingAggregateJob(havingInput)
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range []struct {
		job    PortableJob
		want   []datasource.AggregateOperation
		decode func(string) (datasource.AggregateOperation, error)
	}{
		{projectionJob, projection, func(raw string) (datasource.AggregateOperation, error) {
			return DecodeDatabaseQueryProjectionAggregateLeaf(projectionInput, raw)
		}},
		{havingJob, having, func(raw string) (datasource.AggregateOperation, error) {
			return DecodeDatabaseQueryHavingAggregateLeaf(havingInput, raw)
		}},
	} {
		prompt, err := RenderPortableJob(current.job)
		if err != nil {
			t.Fatal(err)
		}
		maximum, err := PortableResponseMaximumBytesForJob(current.job)
		if err != nil || maximum != 1 {
			t.Fatalf("%s response maximum = %d, %v; want one opaque letter", current.job.Kind, maximum, err)
		}
		for index, want := range current.want {
			id := opaqueModelChoiceID(index)
			if !strings.Contains(prompt, "\n"+id+". ") {
				t.Errorf("%s omitted available aggregate %q", current.job.Kind, want)
			}
			got, err := current.decode(id)
			if err != nil || got != want {
				t.Errorf("%s choice %s = %q, %v; want %q", current.job.Kind, id, got, err, want)
			}
			if want != datasource.AggregateCountRows {
				fields, err := databaseQueryFieldChoices(state, "", databaseQueryAggregateFieldEligible(want))
				if err != nil || len(fields) == 0 {
					t.Fatalf("available aggregate %q has no eligible field: %v", want, err)
				}
				for _, field := range fields {
					if err := validateDatabaseQueryProjection(state, datasource.RelationalProjection{FieldID: field.value, Aggregate: want}); err != nil {
						t.Errorf("available aggregate %q and field %q fail the actual projection validator: %v", want, field.value, err)
					}
				}
			}
		}
		next := opaqueModelChoiceID(len(current.want))
		if strings.Contains(prompt, "\n"+next+". ") {
			t.Errorf("%s rendered an aggregate without a compatible field", current.job.Kind)
		}
		if _, err := current.decode(next); err == nil {
			t.Errorf("%s accepted unavailable aggregate choice %s", current.job.Kind, next)
		}
	}
}
