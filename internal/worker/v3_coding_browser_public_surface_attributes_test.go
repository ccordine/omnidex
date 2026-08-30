package worker

import (
	"strings"
	"testing"
)

func TestBrowserPublicSurfaceRejectsFormsAndSubmitControls(t *testing.T) {
	fixtures := map[string]struct {
		source string
		want   string
	}{
		"form": {
			source: `function View() { return <form><input aria-label="Account" /></form>; }`,
			want:   `unsupported intrinsic element "form"`,
		},
		"implicit button submit": {
			source: `function View() { return <button>Save account</button>; }`,
			want:   `requires exact type="button"`,
		},
		"explicit button submit": {
			source: `function View() { return <button type="submit">Save account</button>; }`,
			want:   `requires exact type="button"`,
		},
		"button reset": {
			source: `function View() { return <button type="reset">Reset account</button>; }`,
			want:   `requires exact type="button"`,
		},
		"case variant button type": {
			source: `function View() { return <button type="BUTTON">Save account</button>; }`,
			want:   `requires exact type="button"`,
		},
		"submit input": {
			source: `function View() { return <input type="submit" aria-label="Save account" />; }`,
			want:   `unsupported input type "submit"`,
		},
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(fixture.source)
			if err == nil || !strings.Contains(err.Error(), fixture.want) {
				t.Fatalf("error=%v, want %q", err, fixture.want)
			}
		})
	}
}

func TestBrowserPublicSurfaceRejectsEffectfulAndUnknownAttributes(t *testing.T) {
	fixtures := map[string]struct {
		source string
		name   string
	}{
		"form action": {
			source: `function View() { return <main action="/send">Send report</main>; }`,
			name:   "action",
		},
		"button form action": {
			source: `function View() { return <button type="button" formAction="/send">Send report</button>; }`,
			name:   "formAction",
		},
		"submit event": {
			source: `function View() { return <main onSubmit={() => void 0}>Send report</main>; }`,
			name:   "onSubmit",
		},
		"unknown event": {
			source: `function View() { return <button type="button" onMouseEnter={() => void 0}>Preview report</button>; }`,
			name:   "onMouseEnter",
		},
		"HTML injection": {
			source: `function View() { return <main dangerouslySetInnerHTML={{ __html: "report" }} />; }`,
			name:   "dangerouslySetInnerHTML",
		},
		"reference": {
			source: `function View() { return <input aria-label="Account" ref={() => void 0} />; }`,
			name:   "ref",
		},
		"style": {
			source: `function View() { return <main style={{ display: "block" }}>Report</main>; }`,
			name:   "style",
		},
		"data attribute": {
			source: `function View() { return <main data-route="private">Report</main>; }`,
			name:   "data-route",
		},
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(fixture.source)
			want := "unsupported attribute " + fixture.name
			if strings.HasPrefix(fixture.name, "on") {
				want = "unsupported event attribute " + fixture.name
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v, want %q", err, want)
			}
		})
	}
}

func TestBrowserPublicSurfaceAcceptsModeledAttributes(t *testing.T) {
	source := `function View() {
  const [query, setQuery] = useState("");
  return <main id="search-panel" className="grid gap-2">
    <label htmlFor="query">Report query</label>
    <input id="query" type="search" placeholder="Quarter" value={query} onInput={(event) => setQuery(event.currentTarget.value)} />
    <label>Report notes<textarea value={query} onChange={(event) => setQuery(event.target.value)} /></label>
    <button type="button" onClick={() => void query}>Run report</button>
    <output aria-label="Current report query">{query}</output>
  </main>;
}`
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(source)
	if err != nil {
		t.Fatalf("modeled safe attributes were rejected: %v", err)
	}
	if len(surface.Controls) != 3 || len(surface.Outputs) != 1 {
		t.Fatalf("unexpected modeled surface: %+v", surface)
	}
}
