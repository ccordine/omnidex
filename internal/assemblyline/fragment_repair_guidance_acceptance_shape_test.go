package assemblyline

import "testing"

func TestFragmentRepairGuidanceAcceptsInstructionWithoutDeclaration(t *testing.T) {
	t.Parallel()
	input := FragmentRepairGuidanceInput{
		Language: "go", Dialect: "Go 1.24 function syntax",
		Signature:          "func Normalize(value string) string",
		CurrentDeclaration: `func Normalize(value string) string { return hidden(value) }`,
		Diagnostic:         `SOURCE_DIAGNOSTIC: undeclared capability "hidden"`,
	}
	job, err := NewFragmentRepairGuidanceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	const instruction = "Replace the call to hidden with the existing value parameter and preserve the return statement."
	guidance, err := DecodeFragmentRepairGuidanceResult(job, instruction)
	if err != nil {
		t.Fatal(err)
	}
	if guidance.Instruction != instruction {
		t.Fatalf("guidance=%q want=%q", guidance.Instruction, instruction)
	}
}

func TestFragmentRepairGuidanceAcceptsSignatureMentionsAndSourceLookingStrings(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name        string
		input       FragmentRepairGuidanceInput
		instruction string
	}{
		{
			name: "Go signature preservation",
			input: FragmentRepairGuidanceInput{
				Language: "go", Dialect: "Go 1.24 function syntax",
				Signature:          "func Normalize(value string) string",
				CurrentDeclaration: `func Normalize(value string) string { return hidden(value) }`,
				Diagnostic:         `SOURCE_DIAGNOSTIC: undeclared capability "hidden"`,
			},
			instruction: "Preserve func Normalize(value string) string exactly; replace hidden(value) with value.",
		},
		{
			name: "Go signature-shaped display text",
			input: FragmentRepairGuidanceInput{
				Language: "go", Dialect: "Go 1.24 function syntax",
				Signature:          "func Label() string",
				CurrentDeclaration: `func Label() string { return "pending" }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: returned display text is incomplete",
			},
			instruction: `Replace the returned literal with "func Normalize(value string) string" and preserve the declaration.`,
		},
		{
			name: "JavaScript signature-shaped display text",
			input: FragmentRepairGuidanceInput{
				Language: "javascript", Dialect: "ECMAScript 2022 function syntax",
				Signature:          "function label()",
				CurrentDeclaration: `function label() { return "pending"; }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: returned display text is incomplete",
			},
			instruction: `Replace the returned literal with "function selectEntry(entries)" and preserve the declaration.`,
		},
		{
			name: "Java signature-shaped display text",
			input: FragmentRepairGuidanceInput{
				Language: "java", Dialect: "Java 21 method syntax",
				Signature:          "public String label()",
				CurrentDeclaration: `public String label() { return "pending"; }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: returned display text is incomplete",
			},
			instruction: `Replace the returned literal with "public int normalize(int value)" and preserve the method.`,
		},
		{
			name: "Rust signature-shaped quoted text",
			input: FragmentRepairGuidanceInput{
				Language: "rust", Dialect: "Rust 2024 function syntax",
				Signature:          "pub fn label() -> String",
				CurrentDeclaration: `pub fn label() -> String { String::from("pending") }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: returned display text is incomplete",
			},
			instruction: `Replace the returned display text with 'pub fn normalize(value: i32) -> i32' and preserve the function.`,
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
			if _, err := DecodeFragmentRepairGuidanceResult(job, fixture.instruction); err != nil {
				t.Fatalf("benign instruction was rejected: %v", err)
			}
		})
	}
}
