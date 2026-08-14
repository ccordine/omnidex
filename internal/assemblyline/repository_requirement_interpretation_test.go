package assemblyline

import (
	"reflect"
	"testing"
)

func TestResolveRepositoryRequirementsGroundsAndSourceSortsAggregate(t *testing.T) {
	t.Parallel()
	input := RepositoryRequirementInterpretationInput{
		UserRequest: "Add audit logging and CSV exports to the service.",
	}
	candidate := RepositoryRequirementInterpretation{
		Schema:        RepositoryRequirementInterpretationSchemaV1,
		FeatureQuotes: []string{"CSV exports", "audit logging"},
	}
	quotes, err := ResolveRepositoryRequirements(input, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(quotes, []string{"audit logging", "CSV exports"}) {
		t.Fatalf("quotes=%q", quotes)
	}
}

func TestResolveRepositoryRequirementsRejectsInvalidAggregate(t *testing.T) {
	t.Parallel()
	input := RepositoryRequirementInterpretationInput{
		UserRequest: "Add audit logging and CSV exports to the service.",
	}
	for name, candidate := range map[string]RepositoryRequirementInterpretation{
		"schema": {Schema: "invalid", FeatureQuotes: []string{"audit logging"}},
		"empty":  {Schema: RepositoryRequirementInterpretationSchemaV1, FeatureQuotes: []string{}},
		"duplicate": {Schema: RepositoryRequirementInterpretationSchemaV1, FeatureQuotes: []string{
			"audit logging", "audit logging",
		}},
		"overlap": {Schema: RepositoryRequirementInterpretationSchemaV1, FeatureQuotes: []string{
			"audit logging", "logging",
		}},
		"ungrounded": {Schema: RepositoryRequirementInterpretationSchemaV1, FeatureQuotes: []string{
			"invented change",
		}},
	} {
		name, candidate := name, candidate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ResolveRepositoryRequirements(input, candidate); err == nil {
				t.Fatalf("accepted invalid aggregate %+v", candidate)
			}
		})
	}
}
