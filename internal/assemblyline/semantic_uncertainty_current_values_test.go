package assemblyline

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestSemanticUncertaintyContainsOnlyCurrentKindAndFiveAnswers(t *testing.T) {
	want := []string{
		"deterministic_consumer", "deterministic_limitation", "exact_question",
		"required_information", "single_result", "work_kind",
	}
	for _, kind := range AllWorkKinds() {
		contract, err := SemanticUncertaintyContractForWorkKind(kind)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(contract)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]string
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		var keys []string
		for key := range fields {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if !slices.Equal(keys, want) || fields["work_kind"] != string(kind) {
			t.Errorf("%s justification contains a duplicate identity or missing answer: %s", kind, encoded)
		}
	}
}

func TestSemanticUncertaintyChecksActualAnswersAndKeepsRegistryUnchanged(t *testing.T) {
	for _, kind := range []WorkKind{WorkFragmentGeneration, WorkGroundedAnswerParagraphInventory} {
		t.Run(string(kind), func(t *testing.T) {
			original, err := SemanticUncertaintyContractForWorkKind(kind)
			if err != nil {
				t.Fatal(err)
			}
			for index := range 5 {
				for _, value := range []string{"", "A different answer."} {
					changed := original
					fields := []*string{
						&changed.ExactQuestion, &changed.DeterministicLimitation,
						&changed.RequiredInformation, &changed.SingleResult,
						&changed.DeterministicConsumer,
					}
					*fields[index] = value
					if err := changed.Validate(); err == nil {
						t.Fatalf("changed answer %d=%q was accepted", index, value)
					}
				}
			}
			current, err := SemanticUncertaintyContractForWorkKind(kind)
			if err != nil || current != original {
				t.Fatalf("editing a returned value changed the current registry: %v", err)
			}
			if err := original.Validate(); err != nil {
				t.Fatalf("exact current answers became unusable: %v", err)
			}
		})
	}
}

func TestSemanticUncertaintyHasNoVersionIdentityOrWordingAuthority(t *testing.T) {
	source, err := os.ReadFile("semantic_uncertainty_contract.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{
		"semanticUncertaintyContractV", "semanticUncertaintyContractIDPrefix",
		"registeredSemanticUncertaintyContractByID", "forbiddenSemanticUncertaintyLanguage",
		"strings.Count(", "strings.HasPrefix(", "strings.HasSuffix(",
	} {
		if strings.Contains(string(source), retired) {
			t.Errorf("semantic justification reintroduced %q", retired)
		}
	}
	if _, err := os.Stat("portable_renderer.go"); !os.IsNotExist(err) {
		t.Fatalf("retired renderer-identity file remains or cannot be checked: %v", err)
	}
}
