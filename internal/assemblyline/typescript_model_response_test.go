package assemblyline

import (
	"strings"
	"testing"
)

func TestDecodeTypeScriptFunctionModelResponseOwnsKnownRawFraming(t *testing.T) {
	tests := []struct {
		name     string
		contract TypeScriptFunctionContract
		raw      string
		want     string
	}{
		{
			name: "single fenced TypeScript declaration",
			contract: TypeScriptFunctionContract{
				Signature: "function total(value: number): number",
			},
			raw: "```typescript\nfunction total(value: number): number { return value + 1; }\n```",
			want: "function total(value: number): number { return value + 1; }",
		},
		{
			name: "single fenced TSX declaration",
			contract: TypeScriptFunctionContract{
				Signature: "async function VerifyCard(): Promise<void>", TSX: true,
			},
			raw: "```tsx\nasync function VerifyCard(): Promise<void> {\n" +
				"  render(<Card />);\n  expect(screen.getByRole('button')).not.toBeNull();\n" +
				"}\n```",
			want: "async function VerifyCard(): Promise<void> {\n" +
				"  render(<Card />);\n  expect(screen.getByRole('button')).not.toBeNull();\n}",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeTypeScriptFunctionModelResponse(test.contract, test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(got) != strings.TrimSpace(test.want) {
				t.Fatalf("decoded response:\n%s\nwant:\n%s", got, test.want)
			}
		})
	}
}

func TestDecodeTypeScriptFunctionModelResponseRejectsExpandedAuthority(t *testing.T) {
	contract := TypeScriptFunctionContract{Signature: "function total(value: number): number"}
	for name, raw := range map[string]string{
		"narrative before fence": "Here is the code:\n```typescript\nfunction total(value: number): number { return value; }\n```",
		"narrative after fence":  "```typescript\nfunction total(value: number): number { return value; }\n```\nDone.",
		"two declarations": "```typescript\nfunction total(value: number): number { return value; }\n" +
			"function other(): number { return 2; }\n```",
		"unknown fence": "```javascript\nfunction total(value: number): number { return value; }\n```",
		"unterminated fence": "```typescript\nfunction total(value: number): number { return value; }",
		"provider controls escaped stop": "function total(value: number): number { return value; }" +
			"<|endoftext|><|im_start|>",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeTypeScriptFunctionModelResponse(contract, raw); err == nil {
				t.Fatal("expanded model response was accepted")
			}
		})
	}
}

func TestDecodeTypeScriptFunctionModelResponsePreservesControlTextInsideDeclaration(t *testing.T) {
	contract := TypeScriptFunctionContract{Signature: "function label(): string"}
	raw := `function label(): string { return "<|endoftext|>"; }`
	got, err := DecodeTypeScriptFunctionModelResponse(contract, raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != raw {
		t.Fatalf("model response changed literal control text: %q", got)
	}
}
