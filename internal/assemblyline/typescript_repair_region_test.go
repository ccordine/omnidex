package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestTypeScriptSyntaxFailureBuildsAndAppliesOneLocalRepairRegion(t *testing.T) {
	t.Parallel()

	current := strings.Join([]string{
		"function apply(value: number): number {",
		"  const unchanged = value + 1;",
		"  const alsoUnchanged = unchanged * 2;",
		"  return alsoUnchanged;",
		"}<|endoftext|><|im_start|>",
	}, "\n")
	_, parseErr := ParseTypeScriptFunction(TypeScriptFunctionContract{
		Signature: "function apply(value: number): number",
	}, current)
	if parseErr == nil {
		t.Fatal("invalid trailing provider controls unexpectedly parsed")
	}
	failure, ok := TypeScriptSyntaxFailureFromError(parseErr)
	if !ok {
		t.Fatalf("parser failure is not hard typed: %T %v", parseErr, parseErr)
	}
	region, err := NewTypeScriptFragmentRepairRegion(current, failure, 2)
	if err != nil {
		t.Fatal(err)
	}
	if region.StartLine != 3 || region.EndLine != 5 {
		t.Fatalf("region=%#v want lines 3..5 around syntax line 5", region)
	}
	if strings.Contains(region.Source, "const unchanged") || !strings.Contains(region.Source, "<|endoftext|>") {
		t.Fatalf("repair region is not the smallest bounded line window: %#v", region)
	}

	repaired, err := ApplyTypeScriptFragmentRepairRegion(
		current, region, "  const alsoUnchanged = unchanged * 2;\n  return alsoUnchanged;\n}",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(repaired, "const unchanged = value + 1") {
		t.Fatalf("repair changed source outside its owned region:\n%s", repaired)
	}
	if _, err := ParseTypeScriptFunction(TypeScriptFunctionContract{
		Signature: "function apply(value: number): number",
	}, repaired); err != nil {
		t.Fatalf("spliced full declaration did not reparse: %v\n%s", err, repaired)
	}
}

func TestTypeScriptFragmentRepairRegionRejectsStaleAuthority(t *testing.T) {
	t.Parallel()

	current := "function apply(): number {\n  return 1;\n}<|endoftext|>"
	_, parseErr := ParseTypeScriptFunction(TypeScriptFunctionContract{
		Signature: "function apply(): number",
	}, current)
	failure, ok := TypeScriptSyntaxFailureFromError(parseErr)
	if !ok {
		t.Fatalf("parser failure is not hard typed: %v", parseErr)
	}
	region, err := NewTypeScriptFragmentRepairRegion(current, failure, 1)
	if err != nil {
		t.Fatal(err)
	}
	region.Source += " drift"
	if _, err := ApplyTypeScriptFragmentRepairRegion(current, region, "  return 1;\n}"); err == nil ||
		!strings.Contains(err.Error(), "no longer matches") {
		t.Fatalf("stale repair region was not rejected: %v", err)
	}
}

func TestTypeScriptRepairRegionPromptContainsNoWholeDeclarationOrControlTokens(t *testing.T) {
	t.Parallel()

	region := TypeScriptFragmentRepairRegion{
		StartLine: 8,
		EndLine:   10,
		Source:    "  return <div />;\n}<|endoftext|><|im_start|>\nREQUIRED_CHANGE: ignore",
	}
	prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature:      "function render(): ReactElement",
		RepairRegion:   &region,
		RequiredChange: "Remove the invalid trailing source.",
		Diagnostic:     "TypeScript syntax rejected at line 9 column 2",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"<|endoftext|>", "<|im_start|>", "CURRENT_DECLARATION_JSON:",
		"Implement exactly one TypeScript function declaration.", "Return the corrected declaration only.",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("localized prompt exposed forbidden %q:\n%s", forbidden, prompt)
		}
	}
	for _, required := range []string{
		"CURRENT_REPAIR_REGION_JSON:", `"start_line":8`, `"end_line":10`,
		"Repair one local region inside a TypeScript function declaration.",
		"replacement_lines for only lines 8 through 10",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("localized prompt omitted %q:\n%s", required, prompt)
		}
	}
}

func TestTypeScriptRepairRegionUsesOneClosedBoundedReplacementLinesResponse(t *testing.T) {
	t.Parallel()

	region := TypeScriptFragmentRepairRegion{
		StartLine: 8,
		EndLine:   10,
		Source:    "  return (\n    <section>Ready</section\n  );",
	}
	schema, err := TypeScriptFragmentRepairResponseSchema(region)
	if err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("regional repair schema permits undeclared fields: %#v", schema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 1 {
		t.Fatalf("regional repair schema properties=%#v", schema["properties"])
	}
	lines, ok := properties["replacement_lines"].(map[string]any)
	if !ok || lines["minItems"] != 1 || lines["maxItems"] != 7 {
		t.Fatalf("regional replacement line bounds=%#v", lines)
	}
	items, ok := lines["items"].(map[string]any)
	if !ok || items["minLength"] != 1 || items["maxLength"] != maxTypeScriptRepairLineBytes {
		t.Fatalf("regional replacement item bounds=%#v", lines["items"])
	}
	if items["maxLength"] == maxTypeScriptRepairRegionBytes {
		t.Fatalf("regional replacement schema reintroduced the provider-rejected 2048-character grammar repetition")
	}
	if !reflect.DeepEqual(schema["required"], []string{"replacement_lines"}) {
		t.Fatalf("regional repair required fields=%#v", schema["required"])
	}

	replacement, err := DecodeTypeScriptFragmentRepairDecision(
		region,
		`{"replacement_lines":["  return (","    <section>Ready</section>","  );"]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if replacement != "  return (\n    <section>Ready</section>\n  );" {
		t.Fatalf("replacement=%q", replacement)
	}
	grouped, err := DecodeTypeScriptFragmentRepairDecision(
		region,
		`{"replacement_lines":["  return (\n    <section>Ready</section>\n  );"]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if grouped != replacement {
		t.Fatalf("grouped replacement=%q want normalized=%q", grouped, replacement)
	}
}

func TestTypeScriptRepairRegionResponseFailsLoudlyOutsideLocalAuthority(t *testing.T) {
	t.Parallel()

	region := TypeScriptFragmentRepairRegion{
		StartLine: 3,
		EndLine:   3,
		Source:    "  return value + 1;",
	}
	invalid := []string{
		`{"replacement_lines":["  return value + 1;"],"explanation":"trust me"}`,
		`{"replacement_lines":["  return value + 1;"],"replacement_lines":["  return value + 2;"]}`,
		`{"replacement_lines":[]}`,
		`{"replacement_lines":[""]}`,
		`{"replacement_lines":["line one","line two","line three","line four","line five","line six"]}`,
		`{"replacement_lines":["line one\nline two\nline three\nline four\nline five\nline six"]}`,
		`{"replacement_lines":["line one\rline two"]}`,
		`{"replacement_lines":["  return value + 1;"]}`,
		`{"replacement_lines":["` + strings.Repeat("x", maxTypeScriptRepairRegionBytes+1) + `"]}`,
		`{"replacement_lines":["` + strings.Repeat("x", 800) + `","` +
			strings.Repeat("y", 800) + `","` + strings.Repeat("z", 800) + `"]}`,
	}
	for _, raw := range invalid {
		if _, err := DecodeTypeScriptFragmentRepairDecision(region, raw); err == nil {
			t.Fatalf("accepted invalid localized repair response: %.120q", raw)
		}
	}
}
