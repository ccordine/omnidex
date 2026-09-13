package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoValueGraphPassesOnlyDirectTypedValues(t *testing.T) {
	requirements := []assemblyline.Requirement{
		{ID: "first"}, {ID: "second"}, {ID: "third"},
	}
	kinds := directCodingResultValueKindPlan{
		"first":  assemblyline.ApplicationResultInteger,
		"second": assemblyline.ApplicationResultText,
		"third":  assemblyline.ApplicationResultBoolean,
	}
	capabilities := directCodingCapabilityGraph{
		"second": {{RequirementID: "first", Purpose: "The supplied quantity."}},
		"third":  {{RequirementID: "second", Purpose: "The supplied description."}},
	}
	sources := directCodingInputSourcePlan{"first": assemblyline.ApplicationInputArguments, "second": assemblyline.ApplicationInputArguments, "third": assemblyline.ApplicationInputArguments}
	indices := goValueIndices(requirements)
	signature, err := goValueSignature("Feature003", "third", indices, capabilities, kinds, sources)
	if err != nil {
		t.Fatal(err)
	}
	if signature != "func Feature003(arguments []string, dependency002 string) bool" {
		t.Fatalf("direct signature=%q", signature)
	}
	order, err := goCommandLineRequirementOrder(requirements, capabilities)
	if err != nil {
		t.Fatal(err)
	}
	lines, blockIDs := goValueInvocationLines(order, indices, capabilities, goValueClosure("third", capabilities), sources)
	want := []string{
		"value001 := Feature001(input001)",
		"value002 := Feature002(input002, value001)",
		"value003 := Feature003(input003, value002)",
	}
	if !sameExactStrings(lines, want) || !sameExactStrings(blockIDs, []string{"feature.001", "feature.002", "feature.003"}) {
		t.Fatalf("invocations=%v; blocks=%v", lines, blockIDs)
	}
	blocks, err := goCommandLineVerificationBlocks(3, "task_003", "third", "Evaluate the description.", "Feature003", indices, capabilities, kinds, order, sources)
	if err != nil {
		t.Fatal(err)
	}
	if blocks[1].Signature != "func ExpectedFeature003(arguments []string, dependency002 string) bool" {
		t.Fatalf("expected-value signature=%q", blocks[1].Signature)
	}
	for _, invocation := range append(want, "expected := ExpectedFeature003(input003, value002)", "if value003 != expected") {
		if !strings.Contains(blocks[2].Static, invocation) {
			t.Fatalf("driver omitted %q: %s", invocation, blocks[2].Static)
		}
	}
	if len(blocks[1].Capabilities) != 0 {
		t.Fatalf("expected-value declaration projection=%v", blocks[1].Capabilities)
	}
}
