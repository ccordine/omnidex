package worker

import (
	"strings"
	"testing"
)

func TestBrowserPublicSurfaceRejectsRuntimeDOMAuthority(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"inventory document mutation": {
			source: `function View() {
  useEffect(() => { document.querySelector("button")?.setAttribute("hidden", ""); }, []);
  return <button type="button">Adjust inventory</button>;
}`,
			want: "runtime host authority identifier document",
		},
		"itinerary window listener": {
			source: `function View() {
  useEffect(() => { window.addEventListener("focus", () => void 0); }, []);
  return <button type="button">Choose itinerary</button>;
}`,
			want: "runtime host authority identifier window",
		},
		"schedule event target alias": {
			source: `function View() {
  return <button type="button" onClick={(event) => {
    const node = event.currentTarget;
    node.setAttribute("hidden", "");
  }}>Publish schedule</button>;
}`,
			want: "event-target authority currentTarget outside read-only value or checked access",
		},
		"billing computed event target": {
			source: `function View() {
  const mutate = (event: unknown) => {
    const targetKey = "target";
    const methodKey = "setAttribute";
    (event as Record<string, any>)[targetKey][methodKey]("hidden", "");
  };
  return <button type="button" onClick={mutate}>Submit payment</button>;
}`,
			want: "unresolved computed event property authority",
		},
		"profile public property mutation": {
			source: `function View() {
  return <button type="button" onClick={(event) => { event.currentTarget.hidden = true; }}>Save profile</button>;
}`,
			want: "runtime DOM event-target property hidden",
		},
		"reservation native event alias": {
			source: `function View() {
  return <button type="button" onClick={(event) => {
    const native = event.nativeEvent;
    void native.target;
  }}>Reserve seat</button>;
}`,
			want: "runtime event property nativeEvent outside target or currentTarget",
		},
		"network shorthand global escape": {
			source: `function View() {
  const authority = { fetch };
  return <button type="button" onClick={() => void authority}>Run check</button>;
}`,
			want: "runtime host authority identifier fetch",
		},
		"event metadata read": {
			source: `function View() {
  return <button type="button" onClick={(event) => { void event.timeStamp; }}>Run check</button>;
}`,
			want: "runtime event property timeStamp outside target or currentTarget",
		},
		"typed constructor reflection": {
			source: `function View() {
  const escape = ({} as unknown as { constructor: { constructor: (source: string) => () => unknown } }).constructor.constructor;
  return <button type="button" onClick={() => void escape("return document")()}>Run check</button>;
}`,
			want: "runtime reflection property constructor",
		},
		"computed prototype reflection": {
			source: `function View() {
  const escape = ({})["__proto__"];
  return <button type="button" onClick={() => void escape}>Run check</button>;
}`,
			want: "runtime reflection property __proto__",
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

func TestBrowserPublicSurfaceAllowsBoundHostNamesAndSafeGlobals(t *testing.T) {
	source := `function View(fetch: (value: string) => string, Worker: (value: string) => string) {
  const [query, setQuery] = useState("");
  const localStorage = new Map<string, string>();
  const indexedDB = { read: (value: string) => value };
	const available = { navigator: "local" };
	const { navigator } = available;
  const handleChange = useCallback((event) => {
    setQuery(String(event.target.value));
  }, []);
	const result = Worker(fetch(indexedDB.read(query))) + navigator;
  localStorage.set("result", result);
  return <main>
    <input aria-label="Search query" value={query} onChange={handleChange} />
    <button type="button" onClick={() => void Math.max(Number(result), 0)}>Run check</button>
    <p>{JSON.stringify(Array.from(localStorage.values()))}</p>
  </main>;
}`
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(source); err != nil {
		t.Fatalf("extract locally bound host names and safe globals: %v", err)
	}
}

func TestBrowserPublicSurfaceDoesNotExtendLocalHostBindingPastScope(t *testing.T) {
	tests := map[string]string{
		"block binding": `function View() {
  {
    const fetch = () => "local";
    void fetch();
  }
  void fetch;
  return <button type="button">Run check</button>;
	}`,
		"function parameter binding": `function View() {
  const invoke = (fetch: () => string) => fetch();
  void invoke;
  void fetch;
  return <button type="button">Run check</button>;
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "runtime host authority identifier fetch") {
				t.Fatalf("want free fetch rejection outside its lexical scope, got %v", err)
			}
		})
	}
}

func TestBrowserPublicSurfaceAllowsReadOnlyControlledInputEventValues(t *testing.T) {
	source := `function View() {
  const [query, setQuery] = useState("");
  const [included, setIncluded] = useState(false);
  return <main>
    <input aria-label="Search query" value={query}
      onChange={(event) => setQuery(event.target.value)} />
    <input aria-label="Include archived" type="checkbox" checked={included}
      onChange={(event) => setIncluded(event.currentTarget.checked)} />
    <button type="button" onClick={() => void query}>Run search</button>
    <p>{query}</p>
  </main>;
}`
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(source); err != nil {
		t.Fatalf("extract controlled-input public surface: %v", err)
	}
}

func TestBrowserPublicSurfaceAllowsUnrelatedIndexedDomainData(t *testing.T) {
	source := `function View() {
  const items = [{ style: "compact", click: "open" }, { style: "wide", click: "closed" }];
  const index = 1;
  const selected = items[index];
  return <main>
    <button type="button" onClick={() => void selected.click}>Choose layout</button>
    <p>{selected.style}</p>
  </main>;
}`
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(source); err != nil {
		t.Fatalf("extract indexed domain-data public surface: %v", err)
	}
}

func TestBrowserPublicSurfaceRejectsUnavailableControlAncestors(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"aria-disabled logistics region": {
			source: `function View() { return <section aria-disabled="true"><button type="button">Dispatch load</button></section>; }`,
			want:   "aria-disabled ancestry",
		},
		"aria-hidden reservation region": {
			source: `function View() { return <section aria-hidden="true"><button type="button">Reserve seat</button></section>; }`,
			want:   "inaccessible controls",
		},
		"inert report region": {
			source: `function View() { return <section inert><input aria-label="Report title" /></section>; }`,
			want:   "inaccessible controls",
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
