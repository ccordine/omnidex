package assemblyline

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseSelectionPromptsIgnoreUnrelatedAcceptedClauses(t *testing.T) {
	for _, fixture := range []struct{ relation, value, timestamp string }{
		{"shipments", "weight", "dispatched_at"},
		{"samples", "reading", "observed_at"},
	} {
		t.Run(fixture.relation, func(t *testing.T) {
			base := databaseSelectionStateFixture(fixture.relation, fixture.value, fixture.timestamp)
			changed := databaseSelectionStateFixture(fixture.relation, fixture.value, fixture.timestamp)
			changeUnrelatedSelectionClauses(&changed)
			if err := changed.validateReady(); err != nil {
				t.Fatalf("changed accepted state is invalid: %v", err)
			}
			before, after := databaseSelectionCases(t, base), databaseSelectionCases(t, changed)
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
						t.Errorf("accepted clause content changed a selection with the same applicable choices\nbefore:\n%s\nafter:\n%s", first, second)
					}
					for _, required := range original.required {
						if !strings.Contains(second, required) {
							t.Errorf("selection omitted required meaning %q", required)
						}
					}
					for _, hidden := range []string{
						"ACCEPTED RESULT SHAPE", "ACCEPTED PROJECTIONS", "ACCEPTED FILTERS",
						"ACCEPTED TEMPORAL WINDOWS", "ACCEPTED EXISTENCE PREDICATES",
						"ACCEPTED HAVING PREDICATES", "ACCEPTED ORDER TERMS", "changed-retained-private-value",
						"need-private", "source-private", base.Authority.ExactNeed,
					} {
						if strings.Contains(second, hidden) {
							t.Errorf("selection exposed unrelated state %q", hidden)
						}
					}
					if err := original.decode(); err != nil {
						t.Errorf("decode original selection: %v", err)
					}
					if err := next.decode(); err != nil {
						t.Errorf("decode selection after unrelated changes: %v", err)
					}
					t.Logf("focused selection input: %d bytes", len(second))
				})
			}
			retained := databaseSelectionStateFixture(fixture.relation, fixture.value, fixture.timestamp)
			changeUnrelatedSelectionClauses(&retained)
			if !reflect.DeepEqual(changed, retained) {
				t.Fatal("rendering or decoding changed code-owned accepted state")
			}
		})
	}
}

func TestDatabaseSelectionPromptsStillUseRelevantChanges(t *testing.T) {
	base := databaseSelectionStateFixture("measurements", "reading", "observed_at")
	changed := databaseSelectionStateFixture("measurements", "voltage", "recorded_at")
	before, after := databaseSelectionCases(t, base), databaseSelectionCases(t, changed)
	for index, original := range before {
		first, err := RenderPortableJob(original.job)
		if err != nil {
			t.Fatal(err)
		}
		second, err := RenderPortableJob(after[index].job)
		if err != nil {
			t.Fatal(err)
		}
		if first == second {
			t.Errorf("%s ignored changed local meaning or available choices", original.job.Kind)
		}
	}
}

func TestDatabaseSelectionPromptStillRejectsInvalidAcceptedState(t *testing.T) {
	state := databaseSelectionStateFixture("measurements", "value", "created_at")
	state.Filters = []datasource.RelationalPredicate{{FieldID: "missing", Operator: datasource.FilterIsNull}}
	for _, current := range databaseSelectionCases(t, state) {
		if _, err := RenderPortableJob(current.job); err == nil {
			t.Errorf("%s rendered against an invalid retained field reference", current.job.Kind)
		}
		if err := current.decode(); err == nil {
			t.Errorf("%s accepted a result against invalid retained state", current.job.Kind)
		}
	}
}
