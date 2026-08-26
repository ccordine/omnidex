package assemblyline

import (
	"strings"
	"testing"
)

func TestBoundedSourceFragmentGenerationRendersOnePathBlindDeclaration(t *testing.T) {
	for _, testCase := range []struct {
		language  string
		signature string
	}{
		{language: "javascript", signature: "function feature(value)"},
		{language: "java", signature: "public int feature()"},
		{language: "rust", signature: "fn feature(value: i32) -> i32"},
		{language: "php", signature: "function feature(int $value): int"},
	} {
		t.Run(testCase.language, func(t *testing.T) {
			t.Parallel()
			input := FragmentGenerationInput{
				Language: testCase.language, Dialect: "qualified test dialect", Signature: testCase.signature,
				Behavior:         "Return the derived value.",
				Capabilities:     []string{"DIRECT_API_DECLARATION"},
				PermittedSymbols: []string{"AllowedSymbol"},
			}
			job, err := NewFragmentGenerationJob(input)
			if err != nil {
				t.Fatal(err)
			}
			prompt, schema, err := RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if schema != nil {
				t.Fatalf("fragment prompt requested schema=%#v", schema)
			}
			for _, exact := range []string{
				testCase.signature, input.Behavior, input.Capabilities[0], input.PermittedSymbols[0],
			} {
				if strings.Count(prompt, exact) != 1 {
					t.Fatalf("prompt did not retain exact input once: %q\n%s", exact, prompt)
				}
			}
			for _, forbidden := range []string{
				"workspace", "target path", "create_file", "write_file", "shell command", "/src/",
			} {
				if strings.Contains(strings.ToLower(prompt), forbidden) {
					t.Fatalf("prompt exposed forbidden authority %q:\n%s", forbidden, prompt)
				}
			}
		})
	}
}

func TestBoundedSourceFragmentGenerationRejectsUnsupportedLanguage(t *testing.T) {
	_, err := BuildBoundedSourceFragmentGenerationPrompt(FragmentGenerationInput{
		Language: "python", Dialect: "qualified test dialect", Signature: "def feature()", Behavior: "Return one.",
	})
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("unsupported language error=%v", err)
	}
}
