package assemblyline

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestComposeGoDocumentBuildsValidatedBlocksAndExactSpans(t *testing.T) {
	t.Parallel()

	document := SourceDocument{
		ID: "feature_source", Path: "feature.go", Preamble: "package main",
		Blocks: []SourceBlock{
			{
				ID: "runtime.api", Static: "type Request struct { Value int }",
				API: "type Request struct { Value int }",
			},
			{
				ID: "feature.001", Signature: "func Feature(request Request) int",
				Contract: "Return the incremented request value.",
				API:      "func Feature(request Request) int",
				DependsOn: []string{
					"runtime.api",
				},
				Capabilities: []string{"runtime.api"},
			},
		},
	}
	composition := SourceComposition{
		Generated: map[string]string{
			"feature.001": "func Feature(request Request) int { return request.Value + 1 }",
		},
		Interfaces: map[string]string{"runtime.api": "type Request struct { Value int }"},
	}
	composed, err := ComposeGoDocument(document, composition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), composed.Path, composed.Source, parser.AllErrors); err != nil {
		t.Fatalf("composed source does not parse: %v\n%s", err, composed.Source)
	}
	if !strings.Contains(composed.Source, "return request.Value + 1") {
		t.Fatalf("composed source=%s", composed.Source)
	}
	if got := composed.Spans["runtime.api"]; got != (SourceSpan{StartLine: 3, EndLine: 3}) {
		t.Fatalf("runtime span=%+v", got)
	}
	if got := composed.Spans["feature.001"]; got != (SourceSpan{StartLine: 5, EndLine: 7}) {
		t.Fatalf("feature span=%+v source=%s", got, composed.Source)
	}
}

func TestComposeGoDocumentUsesOnlyDirectCapabilitiesAndGlobals(t *testing.T) {
	t.Parallel()

	document := SourceDocument{
		ID: "acceptance", Path: "feature_test.go",
		Preamble: "package main\n\nimport \"testing\"",
		Blocks: []SourceBlock{
			{
				ID: "runtime.api", Static: "type Request struct { Value int }",
				API: "type Request struct { Value int }",
			},
			{
				ID: "feature.001", Static: "func Feature(request Request) int { return request.Value + 1 }",
				API: "func Feature(request Request) int", DependsOn: []string{"runtime.api"},
			},
			{
				ID: "acceptance.001", Signature: "func TestFeature(t *testing.T)",
				Contract: "Verify the feature result.", API: "func TestFeature(t *testing.T)",
				DependsOn:    []string{"runtime.api", "feature.001"},
				Capabilities: []string{"runtime.api", "feature.001"}, Globals: []string{"Fatalf"},
			},
		},
	}
	composition := SourceComposition{
		Generated: map[string]string{
			"acceptance.001": `func TestFeature(t *testing.T) {
	if got := Feature(Request{Value: 1}); got != 2 {
		t.Fatalf("got %d", got)
	}
}`,
		},
		Interfaces: map[string]string{
			"runtime.api": "type Request struct { Value int }",
			"feature.001": "func Feature(request Request) int",
		},
	}
	if _, err := ComposeGoDocument(document, composition); err != nil {
		t.Fatal(err)
	}
	delete(composition.Interfaces, "runtime.api")
	if _, err := ComposeGoDocument(document, composition); err == nil || !strings.Contains(err.Error(), "capability runtime.api has no accepted API") {
		t.Fatalf("missing direct capability error=%v", err)
	}
}

func TestComposeGoDocumentRejectsInvalidGeneratedAuthority(t *testing.T) {
	t.Parallel()

	document := generatedGoDocumentFixture()
	for _, testCase := range []struct {
		name      string
		generated map[string]string
		want      string
	}{
		{name: "missing", generated: map[string]string{}, want: "has no source"},
		{name: "signature", generated: map[string]string{"feature.001": "func Renamed() int { return 1 }"}, want: "signature"},
		{name: "undeclared", generated: map[string]string{"feature.001": "func Feature() int { return Hidden() }"}, want: "undeclared capability"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ComposeGoDocument(document, SourceComposition{
				Generated: testCase.generated, Interfaces: map[string]string{},
			})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error=%v want substring %q", err, testCase.want)
			}
		})
	}
}

func TestValidateGoSourceBlueprintRejectsUnsupportedDocumentAuthority(t *testing.T) {
	t.Parallel()

	base := generatedGoDocumentFixture()
	for _, testCase := range []struct {
		name string
		edit func(*SourceDocument)
		want string
	}{
		{name: "path", edit: func(value *SourceDocument) { value.Path = "feature.ts" }, want: "must be Go source"},
		{name: "preamble", edit: func(value *SourceDocument) { value.Preamble = "import \"testing\"" }, want: "preamble"},
		{name: "preamble declaration", edit: func(value *SourceDocument) { value.Preamble = "package main\nvar hidden int" }, want: "only package and import"},
		{name: "export flag", edit: func(value *SourceDocument) { value.Blocks[0].Export = true }, want: "export authority"},
		{name: "policy", edit: func(value *SourceDocument) {
			value.Blocks[0].Policy.ForbiddenIdentifiers = []string{"Hidden"}
		}, want: "policy is unsupported"},
		{name: "signature", edit: func(value *SourceDocument) { value.Blocks[0].Signature = "function Feature(): number" }, want: "parse new Go function signature"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			document := base
			document.Blocks = append([]SourceBlock(nil), base.Blocks...)
			testCase.edit(&document)
			err := ValidateGoSourceBlueprint(SourceBlueprint{Documents: []SourceDocument{document}})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error=%v want substring %q", err, testCase.want)
			}
		})
	}
}

func TestValidateGoSourceBlueprintKeepsPackageAuthorityOutsideScopedPreambles(t *testing.T) {
	t.Parallel()

	document := generatedGoDocumentFixture()
	document.ScopedPreambles = []SourcePreamble{{TaskID: "task_001", Source: `import "testing"`}}
	if err := ValidateGoSourceBlueprint(SourceBlueprint{Documents: []SourceDocument{document}}); err != nil {
		t.Fatal(err)
	}
	document.Preamble = ""
	document.ScopedPreambles[0].Source = "package main"
	err := ValidateGoSourceBlueprint(SourceBlueprint{Documents: []SourceDocument{document}})
	if err == nil || !strings.Contains(err.Error(), "requires a package preamble") {
		t.Fatalf("scoped package authority error=%v", err)
	}
}

func generatedGoDocumentFixture() SourceDocument {
	return SourceDocument{
		ID: "feature", Path: "feature.go", Preamble: "package main",
		Blocks: []SourceBlock{{
			ID: "feature.001", Signature: "func Feature() int",
			Contract: "Return one.", API: "func Feature() int",
		}},
	}
}
