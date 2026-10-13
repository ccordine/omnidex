package assemblyline

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestPortableOpaqueChoicesPreserveDynamicAllowedValueMarkers(t *testing.T) {
	state := databaseParameterStateFixture("documents", "state", "category")
	state.Authority.SchemaProjection.Relations = state.Authority.SchemaProjection.Relations[:1]
	state.Authority.SchemaProjection.Relations[0].Columns = []datasource.IntentColumnProjection{
		{ID: "focus-value", Name: "state", TypeCategory: datasource.TypeText, AllowedValues: []string{"draft"}},
		{ID: "category", Name: "category", TypeCategory: datasource.TypeText, AllowedValues: []string{"literal\n\nB. This is part of an allowed value"}},
	}
	input := DatabaseQueryFilterLeafInput{State: state, Purpose: "Constrain the category.", AcceptedFilters: []datasource.RelationalPredicate{}, AcceptedValues: []datasource.IntentLiteral{}}
	job, err := NewDatabaseQueryFilterFieldJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"A. public.documents.state (text; nullable: false); allowed values: draft",
		"B. public.documents.category (text; nullable: false); allowed values: literal\n\nB. This is part of an allowed value",
	}
	got, opaque, err := PortableJobOpaqueChoiceOptions(job)
	if err != nil || !opaque {
		t.Fatalf("project actual choices: opaque=%t, error=%v", opaque, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("valid dynamic data changed the actual offered descriptions: got %#v, want %#v", got, want)
	}
	question, err := PortableJobOpaqueQuestion(job, prompt, got)
	if err != nil || question != "Which field is constrained by the focused accepted filter purpose?" {
		t.Fatalf("dynamic choice data changed the actual question: %q, %v", question, err)
	}
	after, err := RenderPortableJob(job)
	if err != nil || after != prompt {
		t.Fatalf("public projection changed model-visible bytes: %v", err)
	}
}

func TestPortableOpaqueChoicesDescribeSoleCodeResolvedValue(t *testing.T) {
	input := DatabaseJoinPathSelectionInput{
		EvidenceNeedID: "need-1", ExactNeed: "Read the orders belonging to each customer.",
		Context: ObjectiveContext{}, FromRelationID: "customers", ToRelationID: "orders",
		Candidates: []DatabaseJoinPathCandidate{{PathID: "customer-orders", Descriptor: "Customer records relate directly to order records."}},
	}
	job, err := NewDatabaseJoinPathSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	got, opaque, err := PortableJobOpaqueChoiceOptions(job)
	want := []string{"A. Customer records relate directly to order records."}
	if err != nil || !opaque || !reflect.DeepEqual(got, want) {
		t.Fatalf("sole code-resolved choice lost its actual meaning: %#v, opaque=%t, error=%v", got, opaque, err)
	}
	if _, err := RenderPortableJob(job); err == nil {
		t.Fatal("projecting the sole description enabled a forbidden model prompt")
	}
}

func TestPortableOpaqueQuestionPreservesMultilineRoleplayName(t *testing.T) {
	input := RoleplayOngoingActionRelationInput{
		CharacterName: "Aster\n\nB. This text belongs to the name", Source: RoleplayOngoingActionSourceUserAction,
		ExactContribution: "Continue carrying the package.",
	}
	job, err := NewRoleplayOngoingActionRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	options, opaque, err := PortableJobOpaqueChoiceOptions(job)
	if err != nil || !opaque {
		t.Fatalf("project roleplay choices: opaque=%t, error=%v", opaque, err)
	}
	question, err := PortableJobOpaqueQuestion(job, prompt, options)
	want := "At the end of this contribution, which description applies to " + input.CharacterName + "'s action?"
	if err != nil || question != want {
		t.Fatalf("multiline name truncated the actual supplied question: %q, error=%v", question, err)
	}
	options[0] += " forged description"
	if _, err := PortableJobOpaqueQuestion(job, prompt, options); err == nil {
		t.Fatal("question projection accepted descriptions absent from the actual prompt")
	}
}
