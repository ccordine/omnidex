package assemblyline

import (
	"strings"
	"testing"
)

func TestComposedTypeScriptBlockSourceIsExactAndTerminalNewlineSafe(t *testing.T) {
	t.Parallel()
	document := ComposedSourceDocument{
		ID:     "fixture",
		Source: "const preamble = /outside/;\n\nfunction Inventory(): boolean {\n  return /out of stock/i.test('value');\n}\n\nfunction Schedule(): boolean {\n  return /^monday\\/tuesday$/u.test('value');\n}\n",
		Spans: map[string]SourceSpan{
			"inventory": {StartLine: 3, EndLine: 5},
			"schedule":  {StartLine: 7, EndLine: 9},
		},
	}
	inventory, err := document.BlockSource("inventory")
	if err != nil {
		t.Fatal(err)
	}
	if inventory != "function Inventory(): boolean {\n  return /out of stock/i.test('value');\n}" {
		t.Fatalf("inventory source=%q", inventory)
	}
	schedule, err := document.BlockSource("schedule")
	if err != nil {
		t.Fatal(err)
	}
	if schedule != "function Schedule(): boolean {\n  return /^monday\\/tuesday$/u.test('value');\n}" {
		t.Fatalf("schedule source=%q", schedule)
	}
}

func TestComposedTypeScriptBlockSourceRejectsInvalidAuthority(t *testing.T) {
	t.Parallel()
	for name, document := range map[string]ComposedSourceDocument{
		"missing block": {
			ID: "missing", Source: "const value = 1;", Spans: map[string]SourceSpan{},
		},
		"overlap": {
			ID: "overlap", Source: "const first = 1;\nconst second = 2;\n",
			Spans: map[string]SourceSpan{
				"first": {StartLine: 1, EndLine: 2}, "second": {StartLine: 2, EndLine: 2},
			},
		},
		"synthetic terminal line": {
			ID: "terminal", Source: "const value = 1;\n",
			Spans: map[string]SourceSpan{"first": {StartLine: 2, EndLine: 2}},
		},
		"out of range sibling": {
			ID: "sibling", Source: "const first = 1;\nconst second = 2;\n",
			Spans: map[string]SourceSpan{
				"first": {StartLine: 1, EndLine: 1}, "second": {StartLine: 3, EndLine: 3},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			blockID := "first"
			if name == "missing block" {
				blockID = "absent"
			}
			if _, err := document.BlockSource(blockID); err == nil {
				t.Fatalf("accepted invalid composed authority: %+v", document)
			}
		})
	}
}

func TestComposedTypeScriptBlockSourcePreservesInternalCRLF(t *testing.T) {
	t.Parallel()
	document := ComposedSourceDocument{
		ID: "crlf", Source: "function Verify(): boolean {\r\n  return /ready/i.test('ready');\r\n}\r\n",
		Spans: map[string]SourceSpan{"verify": {StartLine: 1, EndLine: 3}},
	}
	source, err := document.BlockSource("verify")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(source, "\r\n") != 2 || strings.HasSuffix(source, "\r") {
		t.Fatalf("CRLF block source=%q", source)
	}
}
