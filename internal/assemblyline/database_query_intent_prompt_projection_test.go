package assemblyline

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseQueryLeafPromptsExposeOnlyCurrentSelectableOpaqueIDs(t *testing.T) {
	t.Parallel()
	input, clinicID, appointmentClinicID := databaseQueryIntentFixture(t)
	base := NewDatabaseQueryIntentLeafState(input)
	base.FromRelationID = input.SchemaProjection.Relations[0].ID
	base.Shape = datasource.ResultRanking

	var statusID, createdAtID string
	allIDs := []string{
		input.EvidenceNeedID, input.SchemaProjection.SourceID,
		input.SchemaProjection.SchemaFingerprint, input.TemporalAsOf,
	}
	relationIDs := map[string]struct{}{}
	fieldIDs := map[string]struct{}{}
	temporalFieldIDs := map[string]struct{}{}
	for _, relation := range input.SchemaProjection.Relations {
		relationIDs[relation.ID] = struct{}{}
		allIDs = append(allIDs, relation.ID)
		for _, field := range relation.Columns {
			fieldIDs[field.ID] = struct{}{}
			allIDs = append(allIDs, field.ID)
			if field.TypeCategory == datasource.TypeTemporal || field.TypeCategory == datasource.TypeDate {
				temporalFieldIDs[field.ID] = struct{}{}
			}
			if field.Name == "status" {
				statusID = field.ID
			}
			if field.Name == "created_at" {
				createdAtID = field.ID
			}
		}
		for _, foreignKey := range relation.ForeignKeys {
			allIDs = append(allIDs, foreignKey.ID)
		}
	}
	if statusID == "" || createdAtID == "" {
		t.Fatal("database query prompt fixture lacks status or created_at field")
	}

	projectionState := base
	projectionState.Projections = []datasource.RelationalProjection{{FieldID: appointmentClinicID}}
	filterInput := DatabaseQueryFilterLeafInput{
		State: base, AcceptedFilters: []datasource.RelationalPredicate{},
		AcceptedValues: []datasource.IntentLiteral{},
	}
	acceptedFilterInput := filterInput
	acceptedFilterInput.AcceptedFilters = []datasource.RelationalPredicate{{
		FieldID: statusID, Operator: datasource.FilterEqual,
		Values: []datasource.IntentLiteral{{Type: datasource.LiteralString, Value: "open"}},
	}}
	operatorInput := filterInput
	operatorInput.FieldID = statusID
	valueInput := operatorInput
	valueInput.Operator = datasource.FilterIn
	valueInput.AcceptedValues = []datasource.IntentLiteral{{Type: datasource.LiteralString, Value: "open"}}
	windowState := base
	windowState.TemporalWindows = []DatabaseTemporalWindowDecision{{
		FieldID: createdAtID, Unit: datasource.WindowDay, Amount: 7,
	}}
	existenceState := base
	existenceState.Exists = []datasource.ExistencePredicate{{
		RelationID: input.SchemaProjection.Relations[1].ID,
		Filters:    []datasource.RelationalPredicate{},
	}}
	havingState := base
	havingState.Having = []datasource.AggregatePredicate{{
		Aggregate: datasource.AggregateCountRows, Operator: datasource.FilterGT,
		Value: datasource.IntentLiteral{Type: datasource.LiteralInteger, Value: "2"},
	}}
	orderState := base
	orderState.Projections = []datasource.RelationalProjection{
		{FieldID: appointmentClinicID}, {Aggregate: datasource.AggregateCountRows},
	}
	orderState.OrderBy = []datasource.OrderTerm{{Projection: 1, Direction: datasource.OrderDescending}}
	projectionIndex := 0

	type promptCase struct {
		name    string
		build   func() (string, error)
		allowed map[string]struct{}
	}
	cases := []promptCase{
		{name: "from relation", build: func() (string, error) {
			state := NewDatabaseQueryIntentLeafState(input)
			return BuildDatabaseQueryFromRelationPrompt(state)
		}, allowed: relationIDs},
		{name: "shape", build: func() (string, error) {
			state := NewDatabaseQueryIntentLeafState(input)
			state.FromRelationID = base.FromRelationID
			return BuildDatabaseQueryShapePrompt(state)
		}},
		{name: "projection coverage", build: func() (string, error) {
			return BuildDatabaseQueryProjectionCoveragePrompt(projectionState)
		}},
		{name: "projection aggregate", build: func() (string, error) {
			return BuildDatabaseQueryProjectionAggregatePrompt(DatabaseQueryProjectionLeafInput{State: projectionState})
		}},
		{name: "projection field", build: func() (string, error) {
			return BuildDatabaseQueryProjectionFieldPrompt(DatabaseQueryProjectionLeafInput{
				State: base, Aggregate: datasource.AggregateCount,
			})
		}, allowed: fieldIDs},
		{name: "projection time bucket", build: func() (string, error) {
			return BuildDatabaseQueryProjectionTimeBucketPrompt(DatabaseQueryProjectionLeafInput{
				State: base, FieldID: createdAtID,
			})
		}},
		{name: "filter coverage", build: func() (string, error) {
			return BuildDatabaseQueryFilterCoveragePrompt(acceptedFilterInput)
		}},
		{name: "filter field", build: func() (string, error) {
			return BuildDatabaseQueryFilterFieldPrompt(filterInput)
		}, allowed: fieldIDs},
		{name: "filter operator", build: func() (string, error) {
			return BuildDatabaseQueryFilterOperatorPrompt(operatorInput)
		}},
		{name: "filter value coverage", build: func() (string, error) {
			return BuildDatabaseQueryFilterValueCoveragePrompt(valueInput)
		}},
		{name: "filter value", build: func() (string, error) {
			return BuildDatabaseQueryFilterValuePrompt(valueInput)
		}},
		{name: "window coverage", build: func() (string, error) {
			return BuildDatabaseQueryWindowCoveragePrompt(windowState)
		}},
		{name: "window field", build: func() (string, error) {
			return BuildDatabaseQueryWindowFieldPrompt(DatabaseQueryWindowLeafInput{State: base})
		}, allowed: temporalFieldIDs},
		{name: "window unit", build: func() (string, error) {
			return BuildDatabaseQueryWindowUnitPrompt(DatabaseQueryWindowLeafInput{State: base, FieldID: createdAtID})
		}},
		{name: "window amount", build: func() (string, error) {
			return BuildDatabaseQueryWindowAmountPrompt(DatabaseQueryWindowLeafInput{
				State: base, FieldID: createdAtID, Unit: datasource.WindowDay,
			})
		}},
		{name: "existence coverage", build: func() (string, error) {
			return BuildDatabaseQueryExistenceCoveragePrompt(existenceState)
		}},
		{name: "existence relation", build: func() (string, error) {
			return BuildDatabaseQueryExistenceRelationPrompt(DatabaseQueryExistenceLeafInput{
				State: base, Filters: []datasource.RelationalPredicate{},
			})
		}, allowed: relationIDs},
		{name: "existence negated", build: func() (string, error) {
			return BuildDatabaseQueryExistenceNegatedPrompt(DatabaseQueryExistenceLeafInput{
				State: base, RelationID: input.SchemaProjection.Relations[1].ID,
				Filters: []datasource.RelationalPredicate{},
			})
		}},
		{name: "having coverage", build: func() (string, error) {
			return BuildDatabaseQueryHavingCoveragePrompt(havingState)
		}},
		{name: "having aggregate", build: func() (string, error) {
			return BuildDatabaseQueryHavingAggregatePrompt(DatabaseQueryHavingLeafInput{State: base})
		}},
		{name: "having field", build: func() (string, error) {
			return BuildDatabaseQueryHavingFieldPrompt(DatabaseQueryHavingLeafInput{
				State: base, Aggregate: datasource.AggregateCount,
			})
		}, allowed: fieldIDs},
		{name: "having operator", build: func() (string, error) {
			return BuildDatabaseQueryHavingOperatorPrompt(DatabaseQueryHavingLeafInput{
				State: base, Aggregate: datasource.AggregateCountRows,
			})
		}},
		{name: "having value", build: func() (string, error) {
			return BuildDatabaseQueryHavingValuePrompt(DatabaseQueryHavingLeafInput{
				State: base, Aggregate: datasource.AggregateCountRows, Operator: datasource.FilterGT,
			})
		}},
		{name: "order coverage", build: func() (string, error) {
			return BuildDatabaseQueryOrderCoveragePrompt(orderState)
		}},
		{name: "order projection", build: func() (string, error) {
			return BuildDatabaseQueryOrderProjectionPrompt(DatabaseQueryOrderLeafInput{State: orderState})
		}},
		{name: "order direction", build: func() (string, error) {
			return BuildDatabaseQueryOrderDirectionPrompt(DatabaseQueryOrderLeafInput{
				State: orderState, Projection: &projectionIndex,
			})
		}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			prompt, err := test.build()
			if err != nil {
				t.Fatal(err)
			}
			for _, legacy := range []string{
				"(none)", "PROJECTED SCHEMA", "FROM RELATION ID", "CURRENT FILTER FIELD",
				"CURRENT FILTER OPERATOR", "FILTER SCOPE RELATION", "schema_fingerprint",
				"DATABASE QUERY LEAF AUTHORITY",
			} {
				if strings.Contains(prompt, legacy) {
					t.Fatalf("prompt exposed broad or unset builder state %q:\n%s", legacy, prompt)
				}
			}
			selectableIDVisible := len(test.allowed) == 0
			for _, id := range allIDs {
				if _, allowed := test.allowed[id]; allowed {
					selectableIDVisible = selectableIDVisible || strings.Contains(prompt, id)
					continue
				}
				if strings.Contains(prompt, id) {
					t.Fatalf("prompt exposed unrelated code-owned ID %q:\n%s", id, prompt)
				}
			}
			if !selectableIDVisible {
				t.Fatalf("prompt omitted its exact selectable opaque-ID domain:\n%s", prompt)
			}
		})
	}
	if clinicID == appointmentClinicID {
		t.Fatal("database query prompt fixture field identities collapsed")
	}
}

func TestDatabaseQueryPortablePayloadRetainsBindingHiddenFromOperatorPrompt(t *testing.T) {
	t.Parallel()
	input, _, appointmentClinicID := databaseQueryIntentFixture(t)
	state := NewDatabaseQueryIntentLeafState(input)
	state.FromRelationID = input.SchemaProjection.Relations[0].ID
	state.Shape = datasource.ResultRecords
	leaf := DatabaseQueryFilterLeafInput{
		State: state, FieldID: appointmentClinicID,
		AcceptedFilters: []datasource.RelationalPredicate{}, AcceptedValues: []datasource.IntentLiteral{},
	}
	job, err := NewDatabaseQueryFilterOperatorJob(leaf)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(job.Payload), appointmentClinicID) {
		t.Fatalf("portable payload lost code-owned field binding: %s", job.Payload)
	}
	if strings.Contains(prompt, appointmentClinicID) {
		t.Fatalf("operator prompt exposed code-owned focused field ID:\n%s", prompt)
	}
}
