package assemblyline

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTypeScriptFunctionAcceptsOneExactRawTSXDeclaration(t *testing.T) {
	contract := TypeScriptFunctionContract{
		Signature: "function Transport(props: TransportProps): JSX.Element",
		TSX:       true,
	}
	raw := `function Transport(props: TransportProps): JSX.Element {
  return <button type="button" onClick={props.onToggle}>Play</button>;
}`
	fragment, err := ParseTypeScriptFunction(contract, raw)
	if err != nil {
		t.Fatal(err)
	}
	if fragment.Name != "Transport" || !strings.HasSuffix(fragment.Source, "\n") {
		t.Fatalf("fragment=%#v", fragment)
	}
	if fragment.API != contract.Signature {
		t.Fatalf("api=%q want %q", fragment.API, contract.Signature)
	}
}

func TestTypeScriptViolationProvidesTypedCommentCorrectionWithoutPhraseMatching(t *testing.T) {
	t.Parallel()

	contract := TypeScriptFunctionContract{Signature: "function render(): null"}
	_, err := ParseTypeScriptFunction(contract, "function render(): null { /* later */ return null; }")
	if err == nil {
		t.Fatal("comment was accepted")
	}
	instruction, ok := TypeScriptFragmentCorrectionInstruction(err)
	if !ok || !strings.Contains(instruction, "Delete every comment node") || !strings.Contains(instruction, "Change nothing unrelated") {
		t.Fatalf("instruction=%q ok=%v error=%v", instruction, ok, err)
	}
	if _, ok := TypeScriptFragmentCorrectionInstruction(errors.New(err.Error())); ok {
		t.Fatal("plain text resembling a typed violation selected a correction instruction")
	}
}

func TestTypeScriptViolationProvidesTypedEmptyBodyCorrection(t *testing.T) {
	t.Parallel()

	contract := TypeScriptFunctionContract{Signature: "function calculate(): number"}
	_, err := ParseTypeScriptFunction(contract, "function calculate(): number { const run = () => {}; run(); return 1; }")
	if err == nil {
		t.Fatal("empty callback body was accepted")
	}
	instruction, ok := TypeScriptFragmentCorrectionInstruction(err)
	if !ok || !strings.Contains(instruction, "empty function or callback body") {
		t.Fatalf("instruction=%q ok=%v error=%v", instruction, ok, err)
	}
}

func TestParseTypeScriptFunctionRejectsNonRawOrExpandedAuthority(t *testing.T) {
	contract := TypeScriptFunctionContract{Signature: "function clamp(value: number): number"}
	for name, raw := range map[string]string{
		"markdown":       "```ts\nfunction clamp(value: number): number { return value; }\n```",
		"import":         "import { x } from './x';\nfunction clamp(value: number): number { return x(value); }",
		"export":         "export function clamp(value: number): number { return value; }",
		"extra":          "function clamp(value: number): number { return value; }\nconst other = 1;",
		"wrong":          "function clamp(value: string): number { return Number(value); }",
		"broken":         "function clamp(value: number): number { return ;",
		"comment":        "function clamp(value: number): number { /* later */ return value; }",
		"empty callback": "function clamp(value: number): number { const ignored = () => {}; return value; }",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTypeScriptFunction(contract, raw); err == nil {
				t.Fatalf("accepted forbidden fragment %q", raw)
			}
		})
	}
}

func TestTypeScriptBlueprintBuildsDependencyWavesWithoutExposingPaths(t *testing.T) {
	blueprint := TypeScriptBlueprint{Documents: []TypeScriptDocument{
		{
			ID: "domain", Path: "src/domain.ts",
			Blocks: []TypeScriptBlock{{
				ID: "domain.types", Static: "interface Item { id: number }", API: "interface Item { id: number }",
			}},
		},
		{
			ID: "view", Path: "src/View.tsx", Header: "import type { Item } from './domain';",
			Blocks: []TypeScriptBlock{{
				ID: "view.render", Signature: "function View(props: { item: Item }): JSX.Element",
				Contract: "Render the supplied item identifier in a real output element.", API: "function View(props: { item: Item }): JSX.Element",
				DependsOn: []string{"domain.types"}, Capabilities: []string{"domain.types"},
			}},
		},
	}}
	waves, err := blueprint.BuildWaves()
	if err != nil {
		t.Fatal(err)
	}
	if len(waves) != 2 || waves[1][0].Block.ID != "view.render" {
		t.Fatalf("waves=%#v", waves)
	}
}

func TestTypeScriptBlueprintRejectsHalfDefinedGeneratedAuthority(t *testing.T) {
	for name, block := range map[string]TypeScriptBlock{
		"signature only": {ID: "view.render", Signature: "function View(): null", API: "function View(): null"},
		"contract only":  {ID: "view.render", Contract: "Return null.", API: "function View(): null"},
	} {
		t.Run(name, func(t *testing.T) {
			blueprint := TypeScriptBlueprint{Documents: []TypeScriptDocument{{
				ID: "view", Path: "src/View.tsx", Blocks: []TypeScriptBlock{block},
			}}}
			if err := blueprint.Validate(); err == nil || !strings.Contains(err.Error(), "requires both") {
				t.Fatalf("validation error=%v", err)
			}
		})
	}
}

func TestTypeScriptBlueprintRejectsUnscopedModelCapabilities(t *testing.T) {
	for name, block := range map[string]TypeScriptBlock{
		"not a dependency": {
			ID: "view.render", Signature: "function View(): null", Contract: "Return null.",
			API: "function View(): null", Capabilities: []string{"domain.secret"},
		},
		"static authority": {
			ID: "domain.value", Static: "export const value = 1;", API: "const value: 1",
			DependsOn: []string{"domain.secret"}, Capabilities: []string{"domain.secret"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			blueprint := TypeScriptBlueprint{Documents: []TypeScriptDocument{{
				ID: "domain", Path: "src/domain.ts", Blocks: []TypeScriptBlock{
					{ID: "domain.secret", Static: "export const secret = 1;", API: "const secret: 1"},
					block,
				},
			}}}
			if err := blueprint.Validate(); err == nil || !strings.Contains(err.Error(), "capability") {
				t.Fatalf("validation error=%v", err)
			}
		})
	}
}
