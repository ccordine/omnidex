package assemblyline

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestApplicationRequirementInventoryPreservesCleanSourceOrderedLines(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryTestInput(t)
	want := []string{
		"Transforms supplied records.",
		"Reports the transformed result.",
	}
	raw := strings.Join(want, "\n")

	result, err := DecodeApplicationRequirementInventory(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Candidates, want) {
		t.Fatalf("candidates=%q, want %q", result.Candidates, want)
	}
	if result.RawSHA256 != ExactObjectiveContextSHA(raw) {
		t.Fatalf("raw hash=%q, want exact response hash", result.RawSHA256)
	}
}

func TestApplicationRequirementInventoryRejectsStructurallyInvalidResponse(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryTestInput(t)
	clean := "Transforms supplied records."
	tooMany := make([]string, MaxApplicationRequirementInventoryCandidates+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf(
			"Produces governed result %d.",
			index+1,
		)
	}
	fixtures := map[string]string{
		"markdown hard break before independent line": clean + "  \n" +
			"Reports the transformed result.",
		"leading whitespace":               " " + clean,
		"trailing whitespace":              clean + " ",
		"tab before line feed":             clean + "\t\n" + clean,
		"carriage return boundary":         clean + "\r\n" + clean,
		"blank line":                       clean + "\n\n" + clean,
		"mixed registered absence":         clean + "\n" + ApplicationNoRuntimeRequirementCandidates,
		"candidate count above hard bound": strings.Join(tooMany, "\n"),
	}
	for name, raw := range fixtures {
		name, raw := name, raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := DecodeApplicationRequirementInventory(input, raw)
			if err == nil {
				t.Fatalf("accepted structurally invalid inventory: %+v", result)
			}
			if !reflect.DeepEqual(result, ApplicationRequirementInventory{}) {
				t.Fatalf("invalid inventory retained partial authority: %+v", result)
			}
		})
	}
}

func TestApplicationRequirementInventoryAcceptsOnlyExactAbsence(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryTestInput(t)
	result, err := DecodeApplicationRequirementInventory(
		input,
		ApplicationNoRuntimeRequirementCandidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidates == nil || len(result.Candidates) != 0 {
		t.Fatalf("absence candidates=%v, want non-nil empty array", result.Candidates)
	}
	if result.RawSHA256 != ExactObjectiveContextSHA(ApplicationNoRuntimeRequirementCandidates) {
		t.Fatalf("absence raw hash=%q", result.RawSHA256)
	}
}

func TestApplicationRequirementInventoryAcceptsSemanticTextWithoutFrameworkPrefix(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryTestInput(t)
	result, err := DecodeApplicationRequirementInventory(
		input,
		"Transforms supplied records.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Candidates, []string{"Transforms supplied records."}) {
		t.Fatalf("semantic candidates=%q", result.Candidates)
	}
}

func TestApplicationRequirementInventoryPromptHasNoCandidateResponseFrame(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryTestInput(t)
	prompt, err := BuildApplicationRequirementInventoryPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"The finished software ",
		"begin at byte zero",
		"Return only",
		"JSON",
		"FINAL QUESTION",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("inventory prompt leaked response framing %q: %q", forbidden, prompt)
		}
	}
}

func applicationRequirementInventoryTestInput(t testing.TB) ApplicationRequirementInventoryInput {
	t.Helper()
	const request = "Build a record transformer."
	context, err := BootstrapApplicationContext(request)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementInventoryInput{UserRequest: request, Context: context}
}
