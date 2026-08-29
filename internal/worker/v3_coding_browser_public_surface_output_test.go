package worker

import (
	"fmt"
	"strings"
	"testing"
)

func TestBrowserPublicInteractionSurfaceIgnoresNestedFunctionControlFlow(t *testing.T) {
	source := `
function View() {
  const handleRun = () => {
    if (blocked) {
      return;
    }
    throw new Error("callback failure");
  };
  return <button onClick={handleRun}>Run</button>;
}`
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(source)
	if err != nil {
		t.Fatalf("extract surface with nested callback control flow: %v", err)
	}
	if len(surface.Controls) != 1 || surface.Controls[0].Role != "button" ||
		surface.Controls[0].AccessibleName != "Run" {
		t.Fatalf("unexpected nested-callback surface: %+v", surface)
	}
}

func TestBrowserPublicInteractionSurfaceOmitsUnboundTextAuthority(t *testing.T) {
	fixtures := map[string]string{
		"static result":      `function View() { return <main><p>Result: 100</p></main>; }`,
		"inventory quantity": `function View() { return <p>Units available: {quantity}</p>; }`,
		"travel duration":    `function View() { return <p>Suggested duration: {duration}</p>; }`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			surface, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := renderDirectCodingBrowserPublicInteractionSurface(surface)
			if err != nil {
				t.Fatal(err)
			}
			if len(surface.Outputs) != 0 || strings.Contains(receipt, "OUTPUT ") ||
				strings.Contains(receipt, "100") || strings.Contains(receipt, "duration") ||
				strings.Contains(receipt, "quantity") {
				t.Fatalf("unbound text leaked verifier authority:\n%s", receipt)
			}
		})
	}
}

func TestBrowserPublicInteractionSurfaceRejectsInvalidOutputs(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"unlabeled": {
			source: `function View() { return <output>{result}</output>; }`,
			want:   "exact literal aria-label",
		},
		"static": {
			source: `function View() { return <output aria-label="Result">100</output>; }`,
			want:   "direct dynamic-only content",
		},
		"number expression": {
			source: `function View() { return <output aria-label="Result">{42}</output>; }`,
			want:   "not runtime-derived",
		},
		"empty string expression": {
			source: `function View() { return <output aria-label="Result">{''}</output>; }`,
			want:   "not runtime-derived",
		},
		"string expression": {
			source: `function View() { return <output aria-label="Result">{"ready"}</output>; }`,
			want:   "not runtime-derived",
		},
		"null expression": {
			source: `function View() { return <output aria-label="Result">{null}</output>; }`,
			want:   "not runtime-derived",
		},
		"undefined expression": {
			source: `function View() { return <output aria-label="Result">{undefined}</output>; }`,
			want:   "not runtime-derived",
		},
		"boolean expression": {
			source: `function View() { return <output aria-label="Result">{false}</output>; }`,
			want:   "not runtime-derived",
		},
		"static template expression": {
			source: "function View() { return <output aria-label=\"Result\">{`ready`}</output>; }",
			want:   "not runtime-derived",
		},
		"static reducible expression": {
			source: `function View() { return <output aria-label="Result">{String(40 + 2)}</output>; }`,
			want:   "not runtime-derived",
		},
		"mixed": {
			source: `function View() { return <output aria-label="Result">Value: {result}</output>; }`,
			want:   "mixed literal and dynamic content",
		},
		"nested": {
			source: `function View() { return <output aria-label="Result"><span>{result}</span></output>; }`,
			want:   "direct dynamic-only content",
		},
		"self closing": {
			source: `function View() { return <output aria-label="Result" />; }`,
			want:   "direct dynamic-only content",
		},
		"duplicate name": {
			source: `function View() { return <main><output aria-label="Result">{first}</output><output aria-label="Result">{second}</output></main>; }`,
			want:   `repeat accessible name "Result"`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestBrowserPublicInteractionSurfaceAcceptsRuntimeDerivedOutputs(t *testing.T) {
	fixtures := map[string]string{
		"identifier":            `function View() { return <output aria-label="Result">{result}</output>; }`,
		"formatted identifier":  `function View() { return <output aria-label="Result">{String(result)}</output>; }`,
		"member":                `function View() { return <output aria-label="Result">{state.total}</output>; }`,
		"interpolated template": "function View() { return <output aria-label=\"Result\">{`${result}`}</output>; }",
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			surface, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err != nil {
				t.Fatalf("runtime-derived output was rejected: %v", err)
			}
			if len(surface.Outputs) != 1 || surface.Outputs[0].AccessibleName != "Result" {
				t.Fatalf("unexpected runtime output surface: %+v", surface.Outputs)
			}
		})
	}
}

func TestBrowserPublicInteractionSurfaceAccessibleNameSkipsHiddenDescendants(t *testing.T) {
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(
		`function View() { return <button><span aria-hidden="true">Decorative</span>Save item</button>; }`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(surface.Controls) != 1 || surface.Controls[0].AccessibleName != "Save item" {
		t.Fatalf("hidden descendant polluted accessible name: %+v", surface.Controls)
	}
}

func TestBrowserPublicInteractionSurfaceEnforcesBoundsAndCanonicalState(t *testing.T) {
	tooManyControls := `function View() { return <main>` +
		strings.Repeat(`<input aria-label="Value" />`, directCodingBrowserPublicSurfaceMaxControls+1) +
		`</main>; }`
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(tooManyControls); err == nil ||
		!strings.Contains(err.Error(), "exceeds 32 controls") {
		t.Fatalf("control bound was not enforced: %v", err)
	}
	var outputs strings.Builder
	outputs.WriteString(`function View() { return <main>`)
	for index := 0; index <= directCodingBrowserPublicSurfaceMaxOutputs; index++ {
		fmt.Fprintf(&outputs, `<output aria-label="Result %d">{value}</output>`, index)
	}
	outputs.WriteString(`</main>; }`)
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(outputs.String()); err == nil ||
		!strings.Contains(err.Error(), "exceeds 64 outputs") {
		t.Fatalf("output bound was not enforced: %v", err)
	}
	tooLong := fmt.Sprintf(
		`function View() { return <output aria-label="%s">{value}</output>; }`,
		strings.Repeat("x", directCodingBrowserPublicSurfaceMaxLiteralBytes+1),
	)
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(tooLong); err == nil ||
		!strings.Contains(err.Error(), "literal exceeds") {
		t.Fatalf("literal bound was not enforced: %v", err)
	}
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(
		`function View() { return <input aria-label="Query" />; }`,
	)
	if err != nil {
		t.Fatalf("extract canonical-state fixture: %v", err)
	}
	surface.Controls[0].RoleOrdinal = 2
	if _, err := renderDirectCodingBrowserPublicInteractionSurface(surface); err == nil ||
		!strings.Contains(err.Error(), "non-canonical role ordinals") {
		t.Fatalf("renderer accepted non-canonical role state: %v", err)
	}
}
