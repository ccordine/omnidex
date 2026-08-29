package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

const fragmentCorrectionFramingContract = "EXACT_MUTABLE_SOURCE_JSON is collision-safe input framing only. Decode its JSON string to recover the exact mutable source before applying the instruction."

const fragmentCorrectionRawSourceContract = "Return literal source text. Emit source line boundaries as physical LF bytes, never as the two JSON-framing characters backslash and n. Preserve every backslash escape that belongs inside a source-language string, template, character, or regular-expression literal."

func TestFragmentCorrectionPromptRecoversExactDeclarationFromInputFraming(t *testing.T) {
	t.Parallel()

	current := strings.Join([]string{
		"function format(value: string): string {",
		`  const escaped = "line\nnext";`,
		"  const template = `slot\\n`;",
		"  return template + value + escaped;",
		"}",
	}, "\n")
	prompt, err := BuildFragmentCorrectionPrompt(FragmentCorrectionInput{
		CurrentDeclaration: current,
		RepairGuidance:     "Replace the returned template with a normalized template while preserving both source-literal escapes.",
	})
	if err != nil {
		t.Fatal(err)
	}

	assertFragmentCorrectionFramingContract(t, prompt)
	if decoded := decodeFragmentCorrectionMutableSource(t, prompt); decoded != current {
		t.Fatalf("decoded declaration changed exact source bytes:\n got: %q\nwant: %q", decoded, current)
	}
	decoded := decodeFragmentCorrectionMutableSource(t, prompt)
	for _, escaped := range []string{`"line\nnext"`, "`slot\\n`"} {
		if !strings.Contains(current, escaped) || !strings.Contains(decoded, escaped) {
			t.Fatalf("legitimate source-literal escape %q was not preserved through JSON input framing", escaped)
		}
	}
}

func TestFragmentCorrectionPromptRecoversExactRepairRegionFromInputFraming(t *testing.T) {
	t.Parallel()

	source := strings.Join([]string{
		`  const pattern = /\d+\n/;`,
		"  return pattern.source;",
	}, "\n")
	region := &TypeScriptFragmentRepairRegion{
		Kind:      TypeScriptRepairRegionSyntaxWindow,
		StartLine: 4,
		EndLine:   5,
		Source:    source,
	}
	prompt, err := BuildFragmentCorrectionPrompt(FragmentCorrectionInput{
		Language:       "typescript",
		Signature:      "function format(): string",
		RepairRegion:   region,
		RepairGuidance: "Replace the returned expression with one that uses the existing pattern source.",
	})
	if err != nil {
		t.Fatal(err)
	}

	assertFragmentCorrectionFramingContract(t, prompt)
	if decoded := decodeFragmentCorrectionMutableSource(t, prompt); decoded != source {
		t.Fatalf("decoded repair region changed exact source bytes:\n got: %q\nwant: %q", decoded, source)
	}
	if escaped := `/\d+\n/`; !strings.Contains(decodeFragmentCorrectionMutableSource(t, prompt), escaped) {
		t.Fatalf("legitimate regular-expression escape %q was not preserved", escaped)
	}
}

func TestFragmentRepairGuidancePromptRequiresConcreteNonIdenticalSourceDelta(t *testing.T) {
	t.Parallel()

	declaration := TypeScriptRepairGuidanceInput{
		Language:           "typescript",
		Dialect:            "TypeScript function syntax",
		Signature:          "function format(value: string): string",
		CurrentDeclaration: "function format(value: string): string { return value; }",
		Diagnostic:         "SOURCE_DIAGNOSTIC: the returned value has the wrong representation",
	}
	region := TypeScriptFragmentRepairRegion{
		Kind:      TypeScriptRepairRegionSyntaxWindow,
		StartLine: 2,
		EndLine:   2,
		Source:    "  return value + ;",
	}
	localized := declaration
	localized.CurrentDeclaration = ""
	localized.RepairRegion = &region

	for name, input := range map[string]TypeScriptRepairGuidanceInput{
		"declaration":   declaration,
		"repair region": localized,
	} {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			prompt, err := BuildTypeScriptRepairGuidancePrompt(input)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				"Name at least one concrete source-byte change",
				"replacement bytes differ from the bytes being replaced",
				"Never prescribe existing source bytes as their own replacement",
				"byte-identical before/after transformation",
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("repair-guidance prompt omitted %q:\n%s", required, prompt)
				}
			}
		})
	}
}

func assertFragmentCorrectionFramingContract(t *testing.T, prompt string) {
	t.Helper()
	for _, required := range []string{
		fragmentCorrectionFramingContract,
		fragmentCorrectionRawSourceContract,
		"EXACT_MUTABLE_SOURCE_JSON:",
		"REQUIRED_SOURCE_TRANSFORMATION:",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("fragment-correction prompt omitted %q:\n%s", required, prompt)
		}
	}
}

func decodeFragmentCorrectionMutableSource(t *testing.T, prompt string) string {
	t.Helper()
	const prefix = "EXACT_MUTABLE_SOURCE_JSON:\n"
	const suffix = "\n\nREQUIRED_SOURCE_TRANSFORMATION:\n"
	_, after, ok := strings.Cut(prompt, prefix)
	if !ok {
		t.Fatalf("fragment-correction prompt omitted %q", prefix)
	}
	encoded, _, ok := strings.Cut(after, suffix)
	if !ok {
		t.Fatalf("fragment-correction prompt omitted %q after mutable source", suffix)
	}
	var decoded string
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("decode EXACT_MUTABLE_SOURCE_JSON: %v\nencoded=%s", err, encoded)
	}
	return decoded
}
