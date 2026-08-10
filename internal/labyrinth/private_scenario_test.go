package labyrinth

import (
	"bytes"
	"testing"
)

func TestPrivateScenarioRoundTripPreservesExactHostAuthority(t *testing.T) {
	t.Parallel()
	base := testGeneratorConfig(SuiteCombined, 914_771)
	base.Difficulty.WorldSize = 64
	cases, _, err := GenerateScaleFamily(base, []int{64, 6_400})
	if err != nil {
		t.Fatal(err)
	}
	for index, generated := range cases {
		raw, err := generated.ExecutionScenario().MarshalPrivateJSON()
		if err != nil {
			t.Fatal(err)
		}
		restored, err := ParsePrivateScenarioJSON(raw)
		if err != nil {
			t.Fatalf("case %d: %v", index, err)
		}
		repeated, err := restored.MarshalPrivateJSON()
		if err != nil || !bytes.Equal(raw, repeated) || restored.Ref() != generated.ExecutionScenario().Ref() ||
			restored.definition.SHA256() != generated.ExecutionScenario().definition.SHA256() {
			t.Fatalf("case %d private authority changed on round trip", index)
		}
	}
}

func TestPrivateScenarioRejectsDuplicateAndCaseAliasAuthority(t *testing.T) {
	t.Parallel()
	generated, err := Generate(testGeneratorConfig(SuiteRetrieve, 914_772))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := generated.ExecutionScenario().MarshalPrivateJSON()
	if err != nil {
		t.Fatal(err)
	}
	for name, changed := range map[string][]byte{
		"duplicate":  bytes.Replace(raw, []byte(`"schema":`), []byte(`"schema":"duplicate","schema":`), 1),
		"case alias": bytes.Replace(raw, []byte(`"definition_sha256":`), []byte(`"Definition_SHA256":`), 1),
	} {
		if _, err := ParsePrivateScenarioJSON(changed); err == nil {
			t.Fatalf("private scenario accepted %s authority", name)
		}
	}
}
