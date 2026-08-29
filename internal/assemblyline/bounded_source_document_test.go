package assemblyline

import (
	"strings"
	"testing"
)

type boundedSourceFixture struct {
	name      string
	document  SourceDocument
	candidate string
	validate  func(SourceBlueprint) error
	compose   func(SourceDocument, SourceComposition) (ComposedSourceDocument, error)
	fragment  func(string, string) (string, error)
	extra     string
}

func TestBoundedSourceDocumentComposersBuildStaticAndGeneratedBlocks(t *testing.T) {
	for _, fixture := range boundedSourceFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			blueprint := SourceBlueprint{Documents: []SourceDocument{fixture.document}}
			if err := fixture.validate(blueprint); err != nil {
				t.Fatal(err)
			}
			composed, err := fixture.compose(fixture.document, SourceComposition{
				Generated:  map[string]string{"feature.001": fixture.candidate},
				Interfaces: map[string]string{"support.api": fixture.document.Blocks[0].API},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(composed.Source, strings.TrimSpace(fixture.candidate)) {
				t.Fatalf("composed source omitted generated declaration:\n%s", composed.Source)
			}
			if got := composed.Spans["support.api"]; got != (SourceSpan{StartLine: 3, EndLine: 3}) {
				t.Fatalf("static span=%+v source=%s", got, composed.Source)
			}
			if got := composed.Spans["feature.001"]; got != (SourceSpan{StartLine: 5, EndLine: 7}) {
				t.Fatalf("generated span=%+v source=%s", got, composed.Source)
			}
		})
	}
}

func TestBoundedSourceDocumentComposersRejectUnownedGeneratedAuthority(t *testing.T) {
	for _, fixture := range boundedSourceFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			base := SourceComposition{
				Generated:  map[string]string{"feature.001": fixture.candidate},
				Interfaces: map[string]string{"support.api": fixture.document.Blocks[0].API},
			}
			for name, edit := range map[string]func(*SourceComposition){
				"missing source": func(value *SourceComposition) { value.Generated = map[string]string{} },
				"extra declaration": func(value *SourceComposition) {
					value.Generated = map[string]string{"feature.001": fixture.candidate + "\n" + fixture.extra}
				},
				"missing capability": func(value *SourceComposition) {
					value.Interfaces = map[string]string{}
				},
			} {
				t.Run(name, func(t *testing.T) {
					composition := SourceComposition{
						Generated:  copyStringMap(base.Generated),
						Interfaces: copyStringMap(base.Interfaces),
					}
					edit(&composition)
					if _, err := fixture.compose(fixture.document, composition); err == nil {
						t.Fatal("invalid generated authority was accepted")
					}
				})
			}
		})
	}
}

func TestBoundedSourceBlueprintsRejectUnsupportedLanguageAuthority(t *testing.T) {
	for _, fixture := range boundedSourceFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			document := fixture.document
			document.Blocks = append([]SourceBlock(nil), document.Blocks...)
			document.Blocks[1].Policy.ForbiddenIdentifiers = []string{"Hidden"}
			if err := fixture.validate(SourceBlueprint{Documents: []SourceDocument{document}}); err == nil ||
				!strings.Contains(err.Error(), "policy is unsupported") {
				t.Fatalf("unsupported policy error=%v", err)
			}
		})
	}
}

func TestJavaScriptDocumentSupportsCodeOwnedExport(t *testing.T) {
	fixture := boundedSourceFixtures()[0]
	fixture.document.Blocks[1].Export = true
	composed, err := fixture.compose(fixture.document, SourceComposition{
		Generated:  map[string]string{"feature.001": fixture.candidate},
		Interfaces: map[string]string{"support.api": fixture.document.Blocks[0].API},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(composed.Source, "export function Feature(value)") {
		t.Fatalf("code-owned export missing:\n%s", composed.Source)
	}
}

func TestJavaDocumentRequiresCodeOwnedClassWrapper(t *testing.T) {
	fixture := boundedSourceFixtures()[1]
	fixture.document.Postamble = ""
	err := fixture.validate(SourceBlueprint{Documents: []SourceDocument{fixture.document}})
	if err == nil || !strings.Contains(err.Error(), "class preamble") {
		t.Fatalf("Java wrapper error=%v", err)
	}
}

func TestPHPDocumentRequiresCodeOwnedOpeningTag(t *testing.T) {
	fixture := boundedSourceFixtures()[3]
	fixture.document.Preamble = ""
	err := fixture.validate(SourceBlueprint{Documents: []SourceDocument{fixture.document}})
	if err == nil || !strings.Contains(err.Error(), "<?php") {
		t.Fatalf("PHP opening-tag error=%v", err)
	}
}

func copyStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func boundedSourceFixtures() []boundedSourceFixture {
	return []boundedSourceFixture{
		boundedJavaScriptFixture(), boundedJavaFixture(), boundedRustFixture(), boundedPHPFixture(),
	}
}

func boundedSourceDocument(path, preamble, static, api, signature string) SourceDocument {
	return SourceDocument{
		ID: "feature", Path: path, Preamble: preamble,
		Blocks: []SourceBlock{
			{ID: "support.api", Static: static, API: api},
			{ID: "feature.001", Signature: signature, Contract: "Return the derived value.", API: signature,
				DependsOn: []string{"support.api"}, Capabilities: []string{"support.api"}},
		},
	}
}

func boundedJavaScriptFixture() boundedSourceFixture {
	return boundedSourceFixture{
		name: "javascript",
		document: boundedSourceDocument(
			"feature.js", "const OFFSET = 1;", "const BASE = 2;", "const BASE = 2;",
			"function Feature(value)",
		),
		candidate: "function Feature(value) {\n  return value + BASE;\n}",
		validate:  ValidateJavaScriptSourceBlueprint, compose: ComposeJavaScriptDocument,
		fragment: ValidateJavaScriptFragment, extra: "function Extra() {}",
	}
}

func boundedJavaFixture() boundedSourceFixture {
	document := boundedSourceDocument(
		"Feature.java", "public final class Feature {",
		"private static int base() { return 1; }", "private static int base()",
		"public int value()",
	)
	document.Postamble = "}"
	return boundedSourceFixture{
		name:      "java",
		document:  document,
		candidate: "public int value() {\n  return base();\n}",
		validate:  ValidateJavaSourceBlueprint, compose: ComposeJavaDocument,
		fragment: ValidateJavaFragment, extra: "public int extra() { return 0; }",
	}
}

func boundedRustFixture() boundedSourceFixture {
	return boundedSourceFixture{
		name: "rust",
		document: boundedSourceDocument(
			"feature.rs", "use std::cmp;", "const BASE: i32 = 2;", "const BASE: i32 = 2;",
			"pub fn feature(value: i32) -> i32",
		),
		candidate: "pub fn feature(value: i32) -> i32 {\n  value + BASE\n}",
		validate:  ValidateRustSourceBlueprint, compose: ComposeRustDocument,
		fragment: ValidateRustFragment, extra: "fn extra() {}",
	}
}

func boundedPHPFixture() boundedSourceFixture {
	return boundedSourceFixture{
		name: "php",
		document: boundedSourceDocument(
			"feature.php", "<?php", "const BASE = 2;", "const BASE = 2;",
			"function feature(int $value): int",
		),
		candidate: "function feature(int $value): int {\n  return $value + BASE;\n}",
		validate:  ValidatePHPSourceBlueprint, compose: ComposePHPDocument,
		fragment: ValidatePHPFragment, extra: "function extra(): void {}",
	}
}
