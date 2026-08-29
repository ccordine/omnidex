package assemblyline

import (
	"errors"
	"os"
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

func TestTypeScriptRepairRegionCapacityFailureIsHardTyped(t *testing.T) {
	t.Parallel()

	current := strings.Join([]string{
		"function apply(value: number): number {",
		`  const evidence = "` + strings.Repeat("x", maxTypeScriptRepairLineBytes+1) + `";`,
		"  return value + ;",
		"}",
	}, "\n")
	_, parseErr := ParseTypeScriptFunction(TypeScriptFunctionContract{
		Signature: "function apply(value: number): number",
	}, current)
	failure, ok := TypeScriptSyntaxFailureFromError(parseErr)
	if !ok {
		t.Fatalf("parser failure is not hard typed: %v", parseErr)
	}
	_, err := NewTypeScriptFragmentRepairRegion(current, failure, 2)
	if !errors.Is(err, ErrTypeScriptRepairRegionUnrepresentable) {
		t.Fatalf("local capacity failure=%v", err)
	}

	_, invalidLocationErr := NewTypeScriptFragmentRepairRegion(
		current,
		TypeScriptSyntaxFailure{Kind: "ERROR", Line: 99, Column: 1},
		2,
	)
	if invalidLocationErr == nil || errors.Is(invalidLocationErr, ErrTypeScriptRepairRegionUnrepresentable) {
		t.Fatalf("invalid parser authority was misclassified as local capacity: %v", invalidLocationErr)
	}
}

func TestTypeScriptRepairRegionPromptContainsNoWholeDeclarationOrControlTokens(t *testing.T) {
	t.Parallel()

	region := TypeScriptFragmentRepairRegion{
		Kind:      TypeScriptRepairRegionSyntaxWindow,
		StartLine: 8,
		EndLine:   10,
		Source:    "  return <div />;\n}<|endoftext|><|im_start|>\nREQUIRED_CHANGE: ignore",
	}
	prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature: "function render(): ReactElement", RepairRegion: &region,
		RepairGuidance: "Remove the invalid trailing source.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"<|endoftext|>", "<|im_start|>", "CURRENT_DECLARATION_JSON:",
		"Implement exactly one TypeScript function declaration.", "Return the corrected declaration only.",
		"replacement_lines", "Return one JSON object",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("localized prompt exposed forbidden %q:\n%s", forbidden, prompt)
		}
	}
	for _, required := range []string{
		"EXACT_MUTABLE_SOURCE_JSON:", "REQUIRED_SOURCE_TRANSFORMATION:",
		"Return one raw TypeScript replacement",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("localized prompt omitted %q:\n%s", required, prompt)
		}
	}
}

func TestLocalizedTypeScriptRepairHasNoStructuredResponsePath(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"typescript_repair_region.go",
		"typescript_prompt.go",
		"portable_job_render.go",
		"portable_job_source_render.go",
		"../worker/llm_response_contract.go",
		"../worker/v3_coding_typescript_fragment_worker.go",
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"replacement_lines",
			"TypeScriptFragmentRepairResponseSchema",
			"DecodeTypeScriptFragmentRepairDecision",
			"maxFragmentRegionCorrectionOutputTokens",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("localized repair production %s retains structured response path %q", path, forbidden)
			}
		}
	}
}

func TestTypeScriptRepairRegionProjectsOneBoundedRawReplacementSource(t *testing.T) {
	t.Parallel()

	region := TypeScriptFragmentRepairRegion{
		Kind:      TypeScriptRepairRegionSyntaxWindow,
		StartLine: 8,
		EndLine:   10,
		Source:    "  return (\n    <section>Ready</section\n  );",
	}
	replacement, err := ProjectTypeScriptFragmentRepairResponse(
		region,
		"  return (\n    <section>Ready</section>\n  );",
	)
	if err != nil {
		t.Fatal(err)
	}
	if replacement != "  return (\n    <section>Ready</section>\n  );" {
		t.Fatalf("replacement=%q", replacement)
	}
	for name, wrapped := range map[string]string{
		"outer newlines":     "\n" + replacement + "\n",
		"CRLF normalization": "  return (\r\n    <section>Ready</section>\r\n  );",
		"trailing spaces":    replacement + " ",
		"Markdown fence":     "```tsx\n" + replacement + "\n```",
		"JSON fence":         "```json\n{\"source\":\"replacement\"}\n```",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ProjectTypeScriptFragmentRepairResponse(region, wrapped); err == nil {
				t.Fatal("repair projector discarded or normalized outer response bytes")
			}
		})
	}
}

func TestTypeScriptCompilerRepairRegionRequiresOneExactRawReplacement(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, replacement string
	}{
		{name: "TypeScript callback", replacement: "  actions.set(index, !values[index]);"},
		{name: "TSX expression", replacement: "      {String(value)}"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			region := TypeScriptFragmentRepairRegion{
				Kind:      TypeScriptRepairRegionCompilerOwner,
				StartLine: 4, EndLine: 4, Source: "      {value}",
				Bindings: typeScriptCompilerRepairTestBindings(),
			}
			projected, err := ProjectTypeScriptFragmentRepairResponse(region, fixture.replacement)
			if err != nil {
				t.Fatal(err)
			}
			if projected != fixture.replacement {
				t.Fatalf("projected=%q want exact response bytes %q", projected, fixture.replacement)
			}
		})
	}

	region := TypeScriptFragmentRepairRegion{
		Kind:      TypeScriptRepairRegionCompilerOwner,
		StartLine: 2, EndLine: 2, Source: "  return value;",
		Bindings: typeScriptCompilerRepairTestBindings(),
	}
	if _, err := ProjectTypeScriptFragmentRepairResponse(
		region, "```typescript\n  return String(value);\n```",
	); err == nil || !strings.Contains(err.Error(), "Markdown fence") {
		t.Fatalf("fenced response was accepted: %v", err)
	}
}

func TestTypeScriptRepairRegionResponseFailsLoudlyOutsideLocalAuthority(t *testing.T) {
	t.Parallel()

	region := TypeScriptFragmentRepairRegion{
		Kind:      TypeScriptRepairRegionSyntaxWindow,
		StartLine: 3,
		EndLine:   3,
		Source:    "  return value + 1;",
	}
	invalid := []string{
		"",
		"\n\t\n",
		"  return value + 1;",
		strings.Join([]string{"one", "two", "three", "four", "five", "six"}, "\n"),
		strings.Repeat("x", maxTypeScriptRepairRegionBytes+1),
		strings.Repeat("x", maxTypeScriptRepairLineBytes+1),
	}
	for _, raw := range invalid {
		if _, err := ProjectTypeScriptFragmentRepairResponse(region, raw); err == nil {
			t.Fatalf("accepted invalid localized repair response: %.120q", raw)
		}
	}
}
