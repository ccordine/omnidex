package assemblyline

import (
	"strings"
	"testing"
)

func TestProjectTypeScriptFunctionModelResponseExtractsExactUntrustedArtifact(t *testing.T) {
	tests := []struct {
		name     string
		contract TypeScriptFunctionContract
		raw      string
		want     string
	}{
		{
			name: "numeric transformation with reasoning and fence",
			contract: TypeScriptFunctionContract{
				Signature: "function total(value: number): number",
			},
			raw: "I should preserve the input type and return a number.\n\n```typescript\n" +
				"function total(value: number): number { return value + 1; }\n```\nThat is the proposed source.",
			want: "function total(value: number): number { return value + 1; }",
		},
		{
			name: "unrelated TSX verification with surrounding prose",
			contract: TypeScriptFunctionContract{
				Signature: "async function VerifyCard(): Promise<void>", TSX: true,
			},
			raw: "The test should exercise the visible control.\r\n```tsx\r\nasync function VerifyCard(): Promise<void> {\r\n" +
				"  render(<Card />);\r\n  expect(screen.getByRole('button')).not.toBeNull();\r\n" +
				"}\r\n```\r\nNo source outside that declaration is required.",
			want: "async function VerifyCard(): Promise<void> {\r\n" +
				"  render(<Card />);\r\n  expect(screen.getByRole('button')).not.toBeNull();\r\n}",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection, err := ProjectTypeScriptFunctionModelResponse(test.contract, test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if projection.Source != test.want {
				t.Fatalf("projected response:\n%q\nwant:\n%q", projection.Source, test.want)
			}
			if projection.Source != test.raw[projection.StartByte:projection.EndByte] {
				t.Fatal("projected source bytes do not match their exact raw span")
			}
			if projection.RawBytes != len(test.raw) || projection.SourceBytes != len(test.want) ||
				projection.DiscardedBytes != len(test.raw)-len(test.want) {
				t.Fatalf("projection metadata=%+v", projection)
			}
		})
	}
}

func TestProjectTypeScriptFunctionModelResponseFailsOnMissingAmbiguousOrExtraExecutableNode(t *testing.T) {
	contract := TypeScriptFunctionContract{Signature: "function total(value: number): number"}
	for name, raw := range map[string]string{
		"missing required node": "I would add the requested function after checking the types.",
		"ambiguous required node": "function total(value: number): number { return value; }\n" +
			"function total(value: number): number { return value + 1; }",
		"extra function node": "```typescript\nfunction total(value: number): number { return value; }\n" +
			"function helper(): number { return 2; }\n```",
		"extra lexical node": "function total(value: number): number { return value; }\n" +
			"const leakedAuthority = 2;",
		"extra lexical node outside fence": "reasoning\n```typescript\n" +
			"function total(value: number): number { return value; }\n```\nconst leakedAuthority = 2;",
		"extra executable call": "console.log('reasoning');\n" +
			"function total(value: number): number { return value; }",
		"export wrapper": "export function total(value: number): number { return value; }",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ProjectTypeScriptFunctionModelResponse(contract, raw); err == nil {
				t.Fatal("invalid model response was accepted")
			}
		})
	}
}

func TestProjectTypeScriptFunctionModelResponseRetainsUniqueRejectedCandidate(t *testing.T) {
	t.Parallel()

	contract := TypeScriptFunctionContract{Signature: "function adjust(value: number): number"}
	fixtures := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "export wrapper",
			raw:  "reasoning\n```typescript\nexport function adjust(value: number): number { return value + 1; }\n```",
			want: "export function adjust(value: number): number { return value + 1; }",
		},
		{
			name: "extra executable declaration",
			raw: "function adjust(value: number): number { return value + 1; }\n" +
				"const unauthorized = 1;",
			want: "function adjust(value: number): number { return value + 1; }",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			projection, err := ProjectTypeScriptFunctionModelResponse(contract, fixture.raw)
			if err == nil {
				t.Fatal("expected expanded-authority rejection")
			}
			if projection.Source != fixture.want ||
				fixture.raw[projection.StartByte:projection.EndByte] != fixture.want {
				t.Fatalf("rejected projection=%+v want exact candidate %q", projection, fixture.want)
			}
		})
	}
}

func TestProjectTypeScriptFunctionModelResponseRetainsIncompleteFunctionForParserRepair(t *testing.T) {
	t.Parallel()

	raw := "function adjust(value: number): number {\n  return value + 1;"
	projection, err := ProjectTypeScriptFunctionModelResponse(
		TypeScriptFunctionContract{Signature: "function adjust(value: number): number"}, raw,
	)
	if err != nil {
		t.Fatalf("project incomplete candidate: %v", err)
	}
	if projection.Source != raw {
		t.Fatalf("projection=%q want exact incomplete candidate %q", projection.Source, raw)
	}
	if _, err := ParseTypeScriptFunction(
		TypeScriptFunctionContract{Signature: "function adjust(value: number): number"},
		projection.Source,
	); err == nil {
		t.Fatal("incomplete candidate bypassed downstream parser rejection")
	}
}

func TestProjectTypeScriptFunctionModelResponsePreservesControlTextInsideDeclaration(t *testing.T) {
	contract := TypeScriptFunctionContract{Signature: "function label(): string"}
	raw := `function label(): string { return "<|endoftext|>"; }`
	projection, err := ProjectTypeScriptFunctionModelResponse(contract, raw)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Source != raw {
		t.Fatalf("model response changed literal control text: %q", projection.Source)
	}
}

func TestProjectTypeScriptFunctionModelResponseDoesNotCapLongUntrustedNarrative(t *testing.T) {
	t.Parallel()

	contract := TypeScriptFunctionContract{Signature: "function ready(): boolean"}
	source := "function ready(): boolean { return true; }"
	raw := strings.Repeat("Detailed reasoning remains untrusted evidence. ", 4*1024) +
		"\n```ts\n" + source + "\n```"
	if len(raw) <= 128*1024 {
		t.Fatalf("fixture=%d bytes; expected it to exceed the retired candidate cap", len(raw))
	}
	projection, err := ProjectTypeScriptFunctionModelResponse(contract, raw)
	if err != nil {
		t.Fatalf("project long untrusted output: %v", err)
	}
	if projection.Source != source || projection.DiscardedBytes <= 128*1024 {
		t.Fatalf("projection=%+v", projection)
	}
}
