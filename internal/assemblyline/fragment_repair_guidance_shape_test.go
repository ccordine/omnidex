package assemblyline

import (
	"strings"
	"testing"
)

func TestFragmentRepairGuidanceRejectsReplacementSourceAcrossLanguages(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name        string
		input       FragmentRepairGuidanceInput
		instruction string
	}{
		{
			name: "Go spacing and prose prefix",
			input: FragmentRepairGuidanceInput{
				Language: "go", Dialect: "Go 1.24 function syntax",
				Signature:          "func Normalize(value string) string",
				CurrentDeclaration: `func Normalize(value string) string { return hidden(value) }`,
				Diagnostic:         `SOURCE_DIAGNOSTIC: undeclared capability "hidden"`,
			},
			instruction: "Replace the function with:\nfunc Normalize (value string) string {\n\treturn value\n}",
		},
		{
			name: "TypeScript multiline header",
			input: FragmentRepairGuidanceInput{
				Language: "typescript", Dialect: "TypeScript 5.9 function syntax",
				Signature:          "function normalize(value: number): number",
				CurrentDeclaration: `function normalize(value: number): number { return hidden(value); }`,
				Diagnostic:         "TYPESCRIPT_DIAGNOSTIC: Cannot find name 'hidden'.",
			},
			instruction: "Use this declaration instead:\nfunction normalize (\n  value: number\n): number {\n  return value;\n}",
		},
		{
			name: "TSX executable body",
			input: FragmentRepairGuidanceInput{
				Language: "typescript", Dialect: "TypeScript 5.9 TSX function syntax",
				Signature:          "function render(value: string): JSX.Element",
				CurrentDeclaration: `function render(value: string): JSX.Element { return <span>{missing}</span>; }`,
				Diagnostic:         "TYPESCRIPT_DIAGNOSTIC: Cannot find name 'missing'.",
			},
			instruction: "Replace the implementation with:\nfunction render (value: string): JSX.Element {\n  return <span>{value}</span>;\n}",
		},
		{
			name: "JavaScript spacing",
			input: FragmentRepairGuidanceInput{
				Language: "javascript", Dialect: "ECMAScript 2022 function syntax",
				Signature:          "function selectEntry(entries, selected)",
				CurrentDeclaration: `function selectEntry(entries, selected) { return missing(entries); }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: undeclared direct symbol missing",
			},
			instruction: `Use function selectEntry (entries,selected) { return entries[0]; }`,
		},
		{
			name: "Java method",
			input: FragmentRepairGuidanceInput{
				Language: "java", Dialect: "Java 21 method syntax",
				Signature:          "public int normalize(int value)",
				CurrentDeclaration: `public int normalize(int value) { return hidden(value); }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: hidden cannot be resolved",
			},
			instruction: "Replace the method with:\npublic int normalize (\n  int value\n) {\n  return \"ready\".length();\n}",
		},
		{
			name: "Rust function",
			input: FragmentRepairGuidanceInput{
				Language: "rust", Dialect: "Rust 2024 function syntax",
				Signature:          "pub fn normalize(value: i32) -> i32",
				CurrentDeclaration: `pub fn normalize(value: i32) -> i32 { hidden(value) }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: cannot find function hidden",
			},
			instruction: "Replace the function with:\npub fn normalize (\n  value: i32\n) -> i32 {\n  value\n}",
		},
		{
			name: "PHP function",
			input: FragmentRepairGuidanceInput{
				Language: "php", Dialect: "PHP 8.2 function syntax",
				Signature:          "function normalize(int $value): int",
				CurrentDeclaration: `function normalize(int $value): int { return hidden($value); }`,
				Diagnostic:         "SOURCE_DIAGNOSTIC: undefined function hidden",
			},
			instruction: "Replace the function with:\nfunction normalize (\n  int $value\n): int {\n  return $value;\n}",
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
			if err := validateFragmentRepairGuidanceInstruction(
				fixture.input, fixture.instruction,
			); err == nil || !strings.Contains(err.Error(), "complete source declaration") {
				t.Fatalf("parser-backed replacement-source boundary error=%v", err)
			}
			if _, err := DecodeFragmentRepairGuidanceResult(job, fixture.instruction); err == nil {
				t.Fatal("decoder accepted replacement source")
			}
		})
	}
}

func TestFragmentRepairGuidanceRejectsCompleteWrongSignatureDeclaration(t *testing.T) {
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
	if _, err := DecodeFragmentRepairGuidanceResult(
		job, `Instead use func Other () string { return "ready" }.`,
	); err == nil || !strings.Contains(err.Error(), "complete source declaration") {
		t.Fatalf("wrong-signature declaration boundary error=%v", err)
	}
}

func TestFragmentRepairGuidanceRejectsSourceOnlyBodiesAndQuotedDeclarations(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		language    string
		dialect     string
		signature   string
		current     string
		body        string
		bracedBody  string
		declaration string
	}{
		{
			language: "go", dialect: "Go 1.24 function syntax",
			signature: "func Normalize(value string) string",
			current:   `func Normalize(value string) string { return hidden(value) }`,
			body:      "return value", bracedBody: "{ return value }",
			declaration: `func Normalize(value string) string { return value }`,
		},
		{
			language: "typescript", dialect: "TypeScript 5.9 function syntax",
			signature: "function normalize(value: number): number",
			current:   `function normalize(value: number): number { return hidden(value); }`,
			body:      "return value;", bracedBody: "{ return value; }",
			declaration: `function normalize(value: number): number { return value; }`,
		},
		{
			language: "javascript", dialect: "ECMAScript 2022 function syntax",
			signature: "function normalize(value)",
			current:   `function normalize(value) { return hidden(value); }`,
			body:      "return value;", bracedBody: "{ return value; }",
			declaration: `function normalize(value) { return value; }`,
		},
		{
			language: "java", dialect: "Java 21 method syntax",
			signature: "public int normalize(int value)",
			current:   `public int normalize(int value) { return hidden(value); }`,
			body:      "return value;", bracedBody: "{ return value; }",
			declaration: `public int normalize(int value) { return value; }`,
		},
		{
			language: "rust", dialect: "Rust 2024 function syntax",
			signature: "pub fn normalize(value: i32) -> i32",
			current:   `pub fn normalize(value: i32) -> i32 { hidden(value) }`,
			body:      "return value;", bracedBody: "{ return value; }",
			declaration: `pub fn normalize(value: i32) -> i32 { value }`,
		},
		{
			language: "php", dialect: "PHP 8.2 function syntax",
			signature: "function normalize(int $value): int",
			current:   `function normalize(int $value): int { return hidden($value); }`,
			body:      "return $value;", bracedBody: "{ return $value; }",
			declaration: `function normalize(int $value): int { return $value; }`,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.language, func(t *testing.T) {
			t.Parallel()
			input := FragmentRepairGuidanceInput{
				Language: fixture.language, Dialect: fixture.dialect,
				Signature: fixture.signature, CurrentDeclaration: fixture.current,
				Diagnostic: "SOURCE_DIAGNOSTIC: rejected local source",
			}
			job, err := NewFragmentRepairGuidanceJob(input)
			if err != nil {
				t.Fatal(err)
			}
			for label, raw := range map[string]string{
				"raw body":    fixture.body,
				"braced body": fixture.bracedBody,
				"line suffix": "Use this replacement:\n" + fixture.body,
				"fenced body": "Use this:\n```" + fixture.language + "\n" + fixture.body + "\n```",
			} {
				if _, err := DecodeFragmentRepairGuidanceResult(job, raw); err == nil ||
					!strings.Contains(err.Error(), "source-only block") {
					t.Fatalf("%s guidance %q error=%v", label, raw, err)
				}
			}
			quoted := "Replace the declaration with `" + fixture.declaration + "`."
			if _, err := DecodeFragmentRepairGuidanceResult(job, quoted); err == nil ||
				!strings.Contains(err.Error(), "complete source declaration") {
				t.Fatalf("quoted declaration guidance %q error=%v", quoted, err)
			}
		})
	}
}

func TestFragmentRepairGuidanceRejectsFencedDeclarationAfterProse(t *testing.T) {
	t.Parallel()
	input := FragmentRepairGuidanceInput{
		Language: "javascript", Dialect: "ECMAScript 2022 function syntax",
		Signature:          "function selectEntry(entries)",
		CurrentDeclaration: `function selectEntry(entries) { return missing(entries); }`,
		Diagnostic:         "SOURCE_DIAGNOSTIC: undeclared direct symbol missing",
	}
	job, err := NewFragmentRepairGuidanceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	instruction := "Replace the implementation with:\n```javascript\nfunction selectEntry (entries) { return entries[0]; }\n```"
	if _, err := DecodeFragmentRepairGuidanceResult(job, instruction); err == nil ||
		!strings.Contains(err.Error(), "complete source declaration") {
		t.Fatalf("fenced declaration boundary error=%v", err)
	}
}

func TestFragmentRepairGuidanceRejectsEscapedQuotedDeclarations(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		input       FragmentRepairGuidanceInput
		instruction string
	}{
		{
			input: FragmentRepairGuidanceInput{
				Language: "go", Dialect: "Go 1.24 function syntax",
				Signature:          "func Normalize(value string) string",
				CurrentDeclaration: `func Normalize(value string) string { return hidden(value) }`,
				Diagnostic:         `SOURCE_DIAGNOSTIC: undeclared capability "hidden"`,
			},
			instruction: `Replace the declaration with "func Normalize(value string) string { return \"ready\" }".`,
		},
		{
			input: FragmentRepairGuidanceInput{
				Language: "typescript", Dialect: "TypeScript 5.9 function syntax",
				Signature:          "function normalize(value: string): string",
				CurrentDeclaration: `function normalize(value: string): string { return hidden(value); }`,
				Diagnostic:         "TYPESCRIPT_DIAGNOSTIC: Cannot find name 'hidden'.",
			},
			instruction: `Replace the declaration with "function normalize(value: string): string { return \"ready\"; }".`,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.input.Language, func(t *testing.T) {
			t.Parallel()
			job, err := NewFragmentRepairGuidanceJob(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeFragmentRepairGuidanceResult(
				job, fixture.instruction,
			); err == nil || !strings.Contains(err.Error(), "complete source declaration") {
				t.Fatalf("escaped declaration error=%v", err)
			}
		})
	}
}
