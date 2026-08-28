package assemblyline

import "testing"

func TestProjectTypeScriptFunctionModelResponseAcceptsOnlyExactRequiredFunction(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name     string
		contract TypeScriptFunctionContract
		raw      string
	}{
		{
			name: "TypeScript",
			contract: TypeScriptFunctionContract{
				Signature: "function total(value: number): number",
			},
			raw: "function total(value: number): number { return value + 1; }",
		},
		{
			name: "TSX with CRLF inside declaration",
			contract: TypeScriptFunctionContract{
				Signature: "async function VerifyCard(): Promise<void>", TSX: true,
			},
			raw: "async function VerifyCard(): Promise<void> {\r\n" +
				"  render(<Card />);\r\n" +
				"  expect(screen.getByRole('button')).not.toBeNull();\r\n}",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			projection, err := ProjectTypeScriptFunctionModelResponse(
				fixture.contract, fixture.raw,
			)
			if err != nil {
				t.Fatal(err)
			}
			if projection.Source != fixture.raw || projection.StartByte != 0 ||
				projection.EndByte != len(fixture.raw) ||
				projection.RawBytes != len(fixture.raw) ||
				projection.SourceBytes != len(fixture.raw) ||
				projection.DiscardedBytes != 0 ||
				projection.RawSHA256 != projection.SourceSHA256 {
				t.Fatalf("projection=%+v", projection)
			}
		})
	}
}

func TestProjectTypeScriptFunctionModelResponseRejectsEveryOuterByteAndWrapper(t *testing.T) {
	t.Parallel()
	contract := TypeScriptFunctionContract{
		Signature: "function total(value: number): number",
	}
	source := "function total(value: number): number { return value; }"
	for name, raw := range map[string]string{
		"empty":                   "",
		"leading space":           " " + source,
		"trailing newline":        source + "\n",
		"Markdown fence":          "```typescript\n" + source + "\n```",
		"JSON fence":              "```json\n{\"source\":\"value\"}\n```",
		"surrounding prose":       "Here is the source:\n" + source,
		"leading comment":         "// generated source\n" + source,
		"trailing comment":        source + "\n/* generated source */",
		"extra declaration":       source + "\nconst extra = 1;",
		"export wrapper":          "export " + source,
		"wrong required function": "function other(value: number): number { return value; }",
		"malformed declaration":   "function total(value: number): number {",
	} {
		t.Run(name, func(t *testing.T) {
			if projection, err := ProjectTypeScriptFunctionModelResponse(contract, raw); err == nil {
				t.Fatalf("accepted invalid response with projection %+v", projection)
			}
		})
	}
}

func TestProjectTypeScriptFunctionModelResponseLeavesSignaturePolicyValidationDownstream(t *testing.T) {
	t.Parallel()
	contract := TypeScriptFunctionContract{
		Signature: "function total(value: number): number",
	}
	raw := "function total(value: string): string { return value; }"
	projection, err := ProjectTypeScriptFunctionModelResponse(contract, raw)
	if err != nil {
		t.Fatalf("exact response projection unexpectedly assumed signature policy: %v", err)
	}
	if _, err := ParseTypeScriptFunction(contract, projection.Source); err == nil {
		t.Fatal("downstream signature parser accepted a mismatched declaration")
	}
}

func TestProjectTypeScriptFunctionModelResponsePreservesControlTextInsideDeclaration(t *testing.T) {
	t.Parallel()
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
