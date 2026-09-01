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

func TestTypeScriptBodyExtractionSelectsOneDirectModuleCallableWithoutMatchingItsWrapper(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		contract      TypeScriptFunctionContract
		response      string
		expectedBody  string
		forbiddenText []string
	}{
		{
			name: "tsx arrow with a nested callback",
			contract: TypeScriptFunctionContract{
				Signature: "function RenderForecast({ forecast, select }: ForecastProps): ReactElement",
				TSX:       true,
			},
			response: "Here is the component.\n```tsx\n" +
				"import type { FC } from \"react\";\n" +
				"interface VolunteeredProps { readonly unused: string }\n" +
				"const Proposed: FC<VolunteeredProps> = ({ unused }: VolunteeredProps) => {\n" +
				"  const label = forecast ? \"Ready\" : \"Waiting\";\n" +
				"  const handle = () => { select(forecast); };\n" +
				"  return <button onClick={handle}>{label}</button>;\n" +
				"};\nexport default Proposed;\n```\nUsage is intentionally omitted.",
			expectedBody:  "return <button onClick={handle}>{label}</button>;",
			forbiddenText: []string{"import type", "VolunteeredProps", "Proposed", "unused"},
		},
		{
			name: "typescript function expression",
			contract: TypeScriptFunctionContract{
				Signature: "function ScaleReading(value: number, factor: number): number",
			},
			response: "```typescript\n" +
				"type Volunteered = (unused: string) => string;\n" +
				"const Suggested: Volunteered = function (unused: string): string {\n" +
				"  return value * factor;\n" +
				"};\nexport { Suggested };\n```",
			expectedBody:  "return value * factor;",
			forbiddenText: []string{"Volunteered", "Suggested", "unused"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fragment, err := ParseTypeScriptFunctionBody(test.contract, test.response)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(fragment.Source, test.contract.Signature) != 1 ||
				!strings.Contains(fragment.Source, test.expectedBody) {
				t.Fatalf("code-owned TypeScript callable extraction=%q", fragment.Source)
			}
			for _, forbidden := range test.forbiddenText {
				if strings.Contains(fragment.Source, forbidden) {
					t.Fatalf(
						"code-owned TypeScript callable extraction retained %q: %q",
						forbidden,
						fragment.Source,
					)
				}
			}
		})
	}
}

func TestTypeScriptBodyExtractionKeepsAnOrdinaryBodyWithALocalCallable(t *testing.T) {
	t.Parallel()
	const signature = "function RenderChoice({ choose }: ChoiceProps): ReactElement"
	response := "```tsx\n" +
		"const handleChoose = () => { choose(); };\n" +
		"return <button type=\"button\" onClick={handleChoose}>Choose</button>;\n```"
	fragment, err := ParseTypeScriptFunctionBody(
		TypeScriptFunctionContract{Signature: signature, TSX: true},
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"const handleChoose = () => { choose(); };",
		"return <button type=\"button\" onClick={handleChoose}>Choose</button>;",
	} {
		if !strings.Contains(fragment.Source, expected) {
			t.Fatalf("ordinary body lost %q: %q", expected, fragment.Source)
		}
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
			count: "0 direct block-bodied callable candidates",
		},
		{
			name: "module without a callable",
			response: "```typescript\nimport { value } from \"./value\";\n" +
				"export default value;\n```",
			count: "0 direct block-bodied callable candidates",
		},
		{
			name: "two declarations",
			response: "```typescript\nfunction One(): number { return 1; }\n```\n" +
				"```typescript\nfunction Two(): number { return 2; }\n```",
			count: "2 direct block-bodied callable candidates",
		},
		{
			name: "two callables in one module",
			response: "```typescript\n" +
				"const One = () => { return 1; };\n" +
				"const Two = function () { return 2; };\n" +
				"export { One, Two };\n```",
			count: "2 direct block-bodied callable candidates",
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
