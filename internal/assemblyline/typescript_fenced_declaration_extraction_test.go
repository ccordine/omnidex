package assemblyline

import (
	"errors"
	"strings"
	"testing"
)

func TestTypeScriptBodyExtractionSelectsTheOnlyFencedFunctionDeclaration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		signature     string
		response      string
		expectedBody  string
		forbiddenText string
	}{
		{
			name:          "numeric selection",
			signature:     "function Greater(left: number, right: number): number",
			response:      "Implementation:\n```typescript\nfunction Proposed(unused: string): string {\n  return left > right ? left : right;\n}\n```\nUsage:\n```typescript\nconst result = Greater(3, 7);\nconsole.log(result);\n```",
			expectedBody:  "return left > right ? left : right;",
			forbiddenText: "const result",
		},
		{
			name:          "string selection",
			signature:     "function Choose(flag: boolean, left: string, right: string): string",
			response:      "```typescript\nconst chosen = Choose(true, \"first\", \"second\");\n```\n```typescript\nexport function Suggested(value: number): number {\n  return flag ? left : right;\n}\n```",
			expectedBody:  "return flag ? left : right;",
			forbiddenText: "const chosen",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fragment, err := ParseTypeScriptFunctionBody(
				TypeScriptFunctionContract{Signature: test.signature},
				test.response,
			)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(fragment.Source, test.signature) != 1 ||
				!strings.Contains(fragment.Source, test.expectedBody) ||
				strings.Contains(fragment.Source, test.forbiddenText) ||
				strings.Contains(fragment.Source, "Proposed") ||
				strings.Contains(fragment.Source, "Suggested") {
				t.Fatalf("code-owned TypeScript declaration extraction=%q", fragment.Source)
			}
		})
	}
}

func TestTypeScriptBodyExtractionRejectsNonUniqueFencedFunctionDeclarations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		response string
		count    string
	}{
		{
			name: "no declaration",
			response: "```typescript\nconst first = Value();\n```\n" +
				"```typescript\nconsole.log(Value());\n```",
			count: "0 syntactically valid function declaration candidates",
		},
		{
			name: "two declarations",
			response: "```typescript\nfunction One(): number { return 1; }\n```\n" +
				"```typescript\nfunction Two(): number { return 2; }\n```",
			count: "2 syntactically valid function declaration candidates",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseTypeScriptFunctionBody(
				TypeScriptFunctionContract{Signature: "function Value(): number"},
				test.response,
			)
			if err == nil || !strings.Contains(err.Error(), test.count) {
				t.Fatalf("non-unique declaration extraction error=%v", err)
			}
			var defect *SourceBodyDefect
			if errors.As(err, &defect) {
				t.Fatalf("non-unique declaration response acquired correction authority: %v", err)
			}
		})
	}
}
