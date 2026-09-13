package assemblyline

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDatabaseFilterSubsetConsumesOnlyInitiallySoleOrExhaustedChoices(t *testing.T) {
	input := databaseFilterSubsetFixture([]string{"ready"})
	assertDeterministic := func(input DatabaseQueryFilterLeafInput, want string) {
		t.Helper()
		job, err := NewDatabaseQueryFilterValueChoiceJob(input)
		if err != nil {
			t.Fatal(err)
		}
		result, resolved, err := ResolvePortableJobWithoutInference(job)
		if err != nil || !resolved {
			t.Fatalf("deterministic selection resolved=%t error=%v", resolved, err)
		}
		choice, err := DecodeDatabaseQueryFilterValueChoice(input, result.Candidate)
		if err != nil || want == "" && choice.Value != nil || want != "" && (choice.Value == nil || choice.Value.Value != want) {
			t.Fatalf("choice=%+v error=%v", choice, err)
		}
		if _, err := RenderPortableJob(job); err == nil {
			t.Fatal("deterministically resolved choice reached model rendering")
		}
	}
	assertDeterministic(input, "ready")
	input.AcceptedValues = []datasource.IntentLiteral{{Type: datasource.LiteralString, Value: "ready"}}
	assertDeterministic(input, "")
	input.State.Authority.SchemaProjection.Relations[0].Columns[0].AllowedValues = []string{"ready", "done"}
	job, err := NewDatabaseQueryFilterValueChoiceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, resolved, err := ResolvePortableJobWithoutInference(job); err != nil || resolved {
		t.Fatalf("last remaining member requires comparison with absence: resolved=%t error=%v", resolved, err)
	}
	choice, err := DecodeDatabaseQueryFilterValueChoice(input, "B")
	if err != nil || choice.Value != nil {
		t.Fatalf("remaining member was forced into the set: %+v / %v", choice, err)
	}
}

func TestDatabaseFilterSubsetRetiresClosedValueInventoriesAndLiteralCalls(t *testing.T) {
	for _, boolean := range []bool{false, true} {
		input := databaseFilterSubsetFixture([]string{"pending", "finished"})
		if boolean {
			column := &input.State.Authority.SchemaProjection.Relations[0].Columns[0]
			column.TypeCategory, column.AllowedValues = datasource.TypeBoolean, nil
		}
		inventory, err := NewDatabaseQueryPurposeInventoryJob(DatabaseQueryPurposeAuthority{
			State: input.State, Collection: DatabaseQueryFilterValuePurpose, ParentPurpose: input.Purpose,
			FocusedFieldID: input.FieldID, FocusedOperator: input.Operator,
		})
		if err != nil {
			t.Fatal(err)
		}
		literal, err := NewDatabaseQueryFilterValueJob(input)
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range []PortableJob{inventory, literal} {
			if _, err := RenderPortableJob(job); err == nil || !strings.Contains(err.Error(), "closed set-membership") {
				t.Fatalf("obsolete %s call was not rejected: %v", job.Kind, err)
			}
		}
		if _, _, err := ResolveSoleDatabaseQueryFilterValueLeaf(input); err == nil {
			t.Fatal("closed membership used the scalar literal resolver")
		}
	}
}

func TestDatabaseFilterSubsetHasOpaqueBoundedOutputAndNoStateLeak(t *testing.T) {
	input := databaseFilterSubsetFixture([]string{"first", "second", databaseFilterNoAdditionalValue})
	input.AcceptedValues = []datasource.IntentLiteral{{Type: datasource.LiteralString, Value: "first"}}
	before, err := BuildDatabaseQueryFilterValueChoicePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	input.State = withUnrelatedAcceptedQueryClauses(input.State)
	after, err := BuildDatabaseQueryFilterValueChoicePrompt(input)
	if err != nil || before != after {
		t.Fatalf("unrelated retained state changed choice context: %v", err)
	}
	for _, omitted := range []string{"first", "unrelated_", "need-private", "source-private", input.State.Authority.ExactNeed} {
		if strings.Contains(after, omitted) {
			t.Errorf("choice prompt exposes %q", omitted)
		}
	}
	for _, raw := range []string{"first", "second", "A, B", "A\nB", "Z"} {
		if _, err := DecodeDatabaseQueryFilterValueChoice(input, raw); err == nil {
			t.Errorf("accepted non-choice response %q", raw)
		}
	}
	choice, err := DecodeDatabaseQueryFilterValueChoice(input, "B")
	if err != nil || choice.Value == nil || choice.Value.Value != databaseFilterNoAdditionalValue {
		t.Fatalf("a legal literal collided with the code-owned absence: %+v / %v", choice, err)
	}
	job, err := NewDatabaseQueryFilterValueChoiceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if maximum, err := PortableResponseMaximumBytesForJob(job); err != nil || maximum != 1 {
		t.Fatalf("opaque response maximum=%d error=%v", maximum, err)
	}
	if err := (PortableResult{Candidate: "AA"}).ValidateFor(job); err == nil {
		t.Fatal("response exceeded the exact round's bound")
	}
}

func TestDatabaseFilterSubsetFitsMaximumEnumDomainWithAbsence(t *testing.T) {
	values := make([]string, 256)
	for index := range values {
		values[index] = fmt.Sprintf("value-%d", index)
	}
	input := databaseFilterSubsetFixture(values)
	choices, err := databaseQueryFilterValueSubsetChoices(input)
	if err != nil || len(choices) != len(values)+1 {
		t.Fatalf("full enum domain and absence choices=%d error=%v", len(choices), err)
	}
	if _, err := BuildDatabaseQueryFilterValueChoicePrompt(input); err != nil {
		t.Fatal(err)
	}
	if choice, err := DecodeDatabaseQueryFilterValueChoice(input, opaqueModelChoiceID(256)); err != nil || choice.Value != nil {
		t.Fatalf("maximum-domain absence=%+v error=%v", choice, err)
	}
}

func databaseFilterSubsetFixture(values []string) DatabaseQueryFilterLeafInput {
	state := databaseParameterStateFixture("documents", "state", "created_at")
	column := &state.Authority.SchemaProjection.Relations[0].Columns[0]
	column.TypeCategory, column.AllowedValues = datasource.TypeText, values
	return DatabaseQueryFilterLeafInput{
		State: state, Purpose: "Match the requested document states.", FieldID: "focus-value", Operator: datasource.FilterIn,
		AcceptedFilters: []datasource.RelationalPredicate{}, AcceptedValues: []datasource.IntentLiteral{},
	}
}
