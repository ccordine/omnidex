package cognitiongauntlet

import (
	"encoding/json"
	"testing"
)

func TestResolveExtendedScenarioSpecContainsOnlyPublicGeneratorAuthority(t *testing.T) {
	t.Parallel()
	for _, suite := range []Suite{SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder} {
		spec, err := ResolveExtendedScenarioSpecV1(suite, 80_000+uint64(len(suite)), extendedRuntimeBudget())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := GenerateExtendedScenario(spec); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"witness", "oracle", "invalid_rails", "omission_rails"} {
			if containsJSONField(raw, forbidden) {
				t.Fatalf("extended public spec exposed private field %q", forbidden)
			}
		}
	}
}

func TestResolveExtendedScenarioSpecRejectsNonScenarioAndUnavailableSuites(t *testing.T) {
	t.Parallel()
	for _, suite := range []Suite{SuiteRetrieve, SuiteResume, SuiteScale, SuiteTransfer, SuiteRogue} {
		if _, err := ResolveExtendedScenarioSpecV1(suite, 1, extendedRuntimeBudget()); err == nil {
			t.Fatalf("suite %q unexpectedly resolved as an executable extended scenario", suite)
		}
	}
}

func containsJSONField(raw []byte, field string) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	_, found := value[field]
	return found
}
