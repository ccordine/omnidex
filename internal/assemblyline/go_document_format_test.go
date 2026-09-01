package assemblyline

import (
	"go/format"
	"testing"
)

func TestComposeGoDocumentNormalizesCodeOwnedAndGeneratedDeclarations(t *testing.T) {
	document := SourceDocument{
		ID: "feature", Path: "feature.go",
		Preamble: "package main\n\nimport (\n\t\"strings\"\n)",
		Blocks: []SourceBlock{
			{
				ID: "helper", Static: `func helper(value string) string { return strings.TrimSpace(value) }`,
				API: "func helper(value string) string",
			},
			{
				ID: "feature.001", Signature: "func Feature001(value string) string",
				Contract: "Return the normalized value with one suffix.",
				API:      "func Feature001(value string) string", DependsOn: []string{"helper"},
				Capabilities: []string{"helper"}, TaskID: "task_001",
				Role: SourceBlockTaskImplementation,
			},
		},
	}
	composed, err := ComposeGoDocument(document, SourceComposition{
		Generated: map[string]string{
			"feature.001": `func Feature001(value string) string { return helper(value)+"!" }`,
		},
		Interfaces: map[string]string{
			"helper":      "func helper(value string) string",
			"feature.001": "func Feature001(value string) string",
		},
	})
	if err != nil {
		t.Fatalf("compose Go document: %v", err)
	}
	want, err := format.Source([]byte(composed.Source))
	if err != nil {
		t.Fatalf("native formatter rejected composed Go source: %v", err)
	}
	if composed.Source != string(want) {
		t.Fatalf("composed Go source is not in canonical native format:\n%s", composed.Source)
	}
	if span := composed.Spans["feature.001"]; span.StartLine <= composed.Spans["helper"].EndLine {
		t.Fatalf("formatted block spans overlap: %+v", composed.Spans)
	}
}
