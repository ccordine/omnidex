package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFragmentRepairGuidanceSupportsUnrelatedRegisteredLanguages(t *testing.T) {
	t.Parallel()
	fixtures := []FragmentRepairGuidanceInput{
		{
			Language: "go", Dialect: "Go 1.24 function syntax",
			Signature:          "func canonicalRune(value rune) rune",
			CurrentDeclaration: `func canonicalRune(value rune) rune { return unicode.ToUpper(value) }`,
			Diagnostic:         `SOURCE_DIAGNOSTIC: Go fragment references undeclared capability "unicode"`,
		},
		{
			Language: "php", Dialect: "PHP 8.2 function syntax",
			Signature: "function transform(array $values): array",
			Capabilities: []string{
				"function normalize(array $values): array",
			},
			CurrentDeclaration: "function transform(array $values): array { return normalize($value); }",
			Diagnostic:         "SOURCE_DIAGNOSTIC: PHP fragment references undeclared local variable $value",
		},
		{
			Language: "javascript", Dialect: "ECMAScript 2022 function syntax",
			Signature:          "function transform(values)",
			PermittedSymbols:   []string{"values"},
			CurrentDeclaration: "function transform(values) { return value; }",
			Diagnostic:         "SOURCE_DIAGNOSTIC: value is not defined",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Language, func(t *testing.T) {
			t.Parallel()
			job, err := NewFragmentRepairGuidanceJob(fixture)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				"SOURCE_LANGUAGE:\n" + fixture.Language,
				"SOURCE_DIALECT:\n" + fixture.Dialect,
				"REQUIRED_DECLARATION_SIGNATURE:\n" + fixture.Signature,
				"do not provide a complete replacement declaration or source block",
				"DECLARATIONS_AVAILABLE_TO_ANALYZE:",
				"IDENTIFIERS_ALREADY_IN_SCOPE:",
				"The two external-authority lists below are exhaustive.",
				"Do not require imports, package/module declarations, sibling declarations",
				"EXACT_MUTABLE_DECLARATION_JSON:", "EXACT_VALIDATION_FAILURE:",
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("repair guidance omitted %q:\n%s", required, prompt)
				}
			}
			if len(fixture.Capabilities) == 0 &&
				!strings.Contains(prompt, "DECLARATIONS_AVAILABLE_TO_ANALYZE:\n(none)") {
				t.Fatalf("repair guidance hid empty capability authority:\n%s", prompt)
			}
			if len(fixture.PermittedSymbols) == 0 &&
				!strings.Contains(prompt, "IDENTIFIERS_ALREADY_IN_SCOPE:\n(none)") {
				t.Fatalf("repair guidance hid empty symbol authority:\n%s", prompt)
			}
		})
	}
}

func TestFragmentRepairGuidanceAcceptsQuotedUserVisibleSourceEscapes(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name        string
		input       FragmentRepairGuidanceInput
		instruction string
	}{
		{
			name: "shipping receipt",
			input: FragmentRepairGuidanceInput{
				Language: "go", Dialect: "Go 1.24 function syntax",
				Signature:          "func receiptText() string",
				CurrentDeclaration: `func receiptText() string { return "" }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: the returned receipt text is incomplete",
			},
			instruction: `Set the returned string to "Order [pending]\nDispatch:\n  09:30" while preserving the declaration.`,
		},
		{
			name: "status presentation",
			input: FragmentRepairGuidanceInput{
				Language: "javascript", Dialect: "ECMAScript 2022 function syntax",
				Signature:          "function statusText()",
				CurrentDeclaration: `function statusText() { return "Ready"; }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: the displayed status omits its second line",
			},
			instruction: `Replace the returned literal with "Ready\nWaiting" and preserve all other source.`,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			job, err := NewFragmentRepairGuidanceJob(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			guidance, err := DecodeFragmentRepairGuidanceResult(job, fixture.instruction)
			if err != nil {
				t.Fatalf("quoted source escape was rejected: %v", err)
			}
			if guidance.Instruction != fixture.instruction {
				t.Fatalf("guidance instruction=%q", guidance.Instruction)
			}
			execution, err := NewSourceProjectedFragmentCorrectionJob(
				FragmentCorrectionInput{
					CurrentDeclaration: fixture.input.CurrentDeclaration,
					RepairGuidance:     guidance.Instruction,
				},
				fixture.input.Language,
			)
			if err != nil {
				t.Fatalf("accepted guidance did not reach its correction envelope: %v", err)
			}
			executionPrompt, err := RenderPortableJob(execution)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(executionPrompt, guidance.Instruction) {
				t.Fatalf("correction prompt lost exact guidance:\n%s", executionPrompt)
			}
		})
	}
}

func TestFragmentRepairGuidanceRejectsPathsInsideQuotedSource(t *testing.T) {
	t.Parallel()
	job, err := NewFragmentRepairGuidanceJob(FragmentRepairGuidanceInput{
		Language: "go", Dialect: "Go 1.24 function syntax",
		Signature:          "func destination() string",
		CurrentDeclaration: `func destination() string { return "" }`,
		Diagnostic:         "SOURCE_DIAGNOSTIC: the returned destination is invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, instruction := range []string{
		`Set the returned string to "../private/value".`,
		`Set the returned string to "C:\\ProgramData\\private\\value".`,
	} {
		if _, err := DecodeFragmentRepairGuidanceResult(job, instruction); err == nil ||
			!strings.Contains(err.Error(), "filesystem identity") {
			t.Fatalf("quoted path %q error=%v", instruction, err)
		}
	}
}

func TestFragmentRepairExecutionWireContainsOnlyInstructionAndMutableSource(t *testing.T) {
	t.Parallel()
	const current = "function transform(array $values): array { return $values; }"
	const instruction = "Return a reversed copy of $values while preserving the exact declaration."
	job, err := NewSourceProjectedFragmentCorrectionJob(FragmentCorrectionInput{
		CurrentDeclaration: current, RepairGuidance: instruction,
	}, "php")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["current_declaration"] != current ||
		payload["repair_guidance"] != instruction {
		t.Fatalf("repair execution payload=%s", job.Payload)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"SOURCE_DIAGNOSTIC", "SOURCE_DIALECT", "EXACT_SIGNATURE", "DIRECT_CAPABILITIES",
		"LOCAL_BEHAVIOR", "workspace", "command", "task", "test source",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("repair execution leaked %q:\n%s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, current) || !strings.Contains(prompt, instruction) {
		t.Fatalf("repair execution lost exact authority:\n%s", prompt)
	}
}

func TestFragmentRepairGuidancePromptExcludesBehaviorAndOperationalAuthority(t *testing.T) {
	t.Parallel()
	input := FragmentRepairGuidanceInput{
		Language: "php", Dialect: "PHP 8.2 function syntax",
		Signature:          "function transform(array $values): array",
		CurrentDeclaration: "function transform(array $values): array { return $values; }",
		Diagnostic:         "SOURCE_DIAGNOSTIC: returned element type is invalid",
	}
	prompt, err := BuildFragmentRepairGuidancePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"LOCAL_BEHAVIOR", "acceptance criteria", "workspace", "filename", "docker compose",
		"queue", "workflow", "task id", "tests/", "src/",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("repair guidance leaked %q:\n%s", forbidden, prompt)
		}
	}
}
