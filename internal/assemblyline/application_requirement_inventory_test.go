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
		"The finished software transforms supplied records.",
		"The finished software reports the transformed result.",
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
	clean := "The finished software transforms supplied records."
	tooMany := make([]string, MaxApplicationRequirementInventoryCandidates+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf(
			"The finished software produces governed result %d.",
			index+1,
		)
	}
	fixtures := map[string]string{
		"markdown hard break before independent line": clean + "  \n" +
			"The finished software reports the transformed result.",
		"leading whitespace":               " " + clean,
		"trailing whitespace":              clean + " ",
		"tab before line feed":             clean + "\t\n" + clean,
		"carriage return boundary":         clean + "\r\n" + clean,
		"blank line":                       clean + "\n\n" + clean,
		"mixed registered absence":         clean + "\n" + ApplicationNoRuntimeRequirementCandidates,
		"missing candidate frame":          "Transforms supplied records.",
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

func TestApplicationRequirementInventoryValidationEnforcesCandidateFrame(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryTestInput(t)
	result, err := DecodeApplicationRequirementInventory(
		input,
		"The finished software transforms supplied records.",
	)
	if err != nil {
		t.Fatal(err)
	}
	result.Candidates = []string{"Transforms supplied records."}
	result.RawSHA256 = ExactObjectiveContextSHA(result.Candidates[0])
	if err := result.ValidateFor(input); err == nil {
		t.Fatal("typed inventory state bypassed the exact candidate frame")
	}
}

func TestApplicationRequirementInventoryPromptForbidsLineBoundaryWhitespace(t *testing.T) {
	t.Parallel()
	input := applicationRequirementInventoryTestInput(t)
	prompt, err := BuildApplicationRequirementInventoryPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"begin at byte zero",
		"end immediately after its last non-whitespace byte",
		"do not use Markdown hard-break spaces before a line feed",
	} {
		if !strings.Contains(strings.ToLower(prompt), strings.ToLower(required)) {
			t.Fatalf("inventory prompt omits %q", required)
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
