package worker

import (
	"strings"
	"testing"
)

func TestBrowserPublicInteractionSurfaceInventory(t *testing.T) {
	source := `
export function InventoryPanel(): JSX.Element {
  const [sku, inventoryStateSetter] = useState("");
  const [quantity, quantitySetter] = useState(0);
  return (
    <main className="grid gap-4">
      <h1>Inventory adjustment</h1>
      <label htmlFor="sku">Stock keeping unit</label>
      <input id="sku" placeholder="ABC-123" value={sku}
        onChange={(event) => inventoryStateSetter(event.target.value)} />
      <label>Quantity
        <input type="number" value={quantity}
          onChange={(event) => quantitySetter(Number(event.target.value))} />
      </label>
      <button type="button" onClick={() => void quantity}>Apply adjustment</button>
      {quantity !== null && <div><span>Current count:</span><output aria-label="Current count">{quantity}</output></div>}
    </main>
  );
}`
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(source)
	if err != nil {
		t.Fatalf("extract inventory public surface: %v", err)
	}
	receipt, err := renderDirectCodingBrowserPublicInteractionSurface(surface)
	if err != nil {
		t.Fatalf("render inventory public surface: %v", err)
	}
	want := `PUBLIC_INTERACTION_SURFACE_V1
CONTROL 1 role=textbox role_ordinal=1 role_count=1 accessible_name="Stock keeping unit" placeholder_hint="ABC-123" value_kind=text
CONTROL 2 role=spinbutton role_ordinal=1 role_count=1 accessible_name="Quantity" placeholder_hint=NONE value_kind=number
CONTROL 3 role=button role_ordinal=1 role_count=1 accessible_name="Apply adjustment" placeholder_hint=NONE value_kind=action
OUTPUT 1 role=status accessible_name="Current count"
END_PUBLIC_INTERACTION_SURFACE`
	if receipt != want {
		t.Fatalf("inventory receipt mismatch\nwant:\n%s\n\ngot:\n%s", want, receipt)
	}
	for _, private := range []string{"InventoryPanel", "inventoryStateSetter", "quantitySetter", "event.target", "onChange"} {
		if strings.Contains(receipt, private) {
			t.Fatalf("receipt leaked private source bytes %q: %s", private, receipt)
		}
	}
}

func TestBrowserPublicInteractionSurfaceTravel(t *testing.T) {
	source := `
export function TravelSearch(): JSX.Element {
	const [departureValueSecret, setDepartureValueSecret] = useState("");
	const [arrivalValueSecret, setArrivalValueSecret] = useState("");
	const [durationValueSecret, setDurationValueSecret] = useState(0);
  return <section>
    <input aria-label="Departure city" placeholder="Boston" value={departureValueSecret}
      onChange={(event) => setDepartureValueSecret(event.target.value)} />
    <label htmlFor="arrival">Arrival city</label>
    <input id="arrival" value={arrivalValueSecret}
      onChange={(event) => setArrivalValueSecret(event.target.value)} />
    <label>Travel class<select value="economy">
      <option value="economy">Economy</option>
      <option value="business">Business</option>
    </select></label>
		<button type="button" onClick={() => setDurationValueSecret(departureValueSecret.length + arrivalValueSecret.length)}><span>Find routes</span></button>
    <p><span>Suggested duration:</span><output aria-label="Suggested duration">{durationValueSecret}</output></p>
	</section>;
}`
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(source)
	if err != nil {
		t.Fatalf("extract travel public surface: %v", err)
	}
	receipt, err := renderDirectCodingBrowserPublicInteractionSurface(surface)
	if err != nil {
		t.Fatalf("render travel public surface: %v", err)
	}
	wantControls := []string{
		`CONTROL 1 role=textbox role_ordinal=1 role_count=2 accessible_name="Departure city" placeholder_hint="Boston" value_kind=text`,
		`CONTROL 2 role=textbox role_ordinal=2 role_count=2 accessible_name="Arrival city" placeholder_hint=NONE value_kind=text`,
		`CONTROL 3 role=combobox role_ordinal=1 role_count=1 accessible_name="Travel class" placeholder_hint=NONE value_kind=selection`,
		`CONTROL 4 role=button role_ordinal=1 role_count=1 accessible_name="Find routes" placeholder_hint=NONE value_kind=action`,
	}
	for _, want := range wantControls {
		if !strings.Contains(receipt, want) {
			t.Fatalf("travel receipt missing %q: %s", want, receipt)
		}
	}
	for _, want := range []string{
		`OUTPUT 1 role=status accessible_name="Suggested duration"`,
	} {
		if !strings.Contains(receipt, want) {
			t.Fatalf("travel receipt missing %q: %s", want, receipt)
		}
	}
	for _, private := range []string{"TravelSearch", "departureValueSecret", "arrivalValueSecret", "durationValueSecret", "onClick"} {
		if strings.Contains(receipt, private) {
			t.Fatalf("receipt leaked private source bytes %q: %s", private, receipt)
		}
	}
}

func TestBrowserPublicInteractionSurfaceRejectsUnprovenSemantics(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"invalid TSX": {
			source: `function View() { return <main>; }`,
			want:   "not valid TSX",
		},
		"spread attributes": {
			source: `function View() { return <input {...props} />; }`,
			want:   "spread attributes",
		},
		"dynamic accessible name": {
			source: `function View() { return <input aria-label={caption} />; }`,
			want:   "attribute aria-label requires an exact literal",
		},
		"custom component": {
			source: `function View() { return <InventoryControl />; }`,
			want:   "custom component",
		},
		"embedded frame": {
			source: `function View() { return <iframe />; }`,
			want:   `unsupported intrinsic element "iframe"`,
		},
		"embedded object": {
			source: `function View() { return <object />; }`,
			want:   `unsupported intrinsic element "object"`,
		},
		"unknown lowercase element": {
			source: `function View() { return <widget />; }`,
			want:   `unsupported intrinsic element "widget"`,
		},
		"ambiguous labels": {
			source: `function View() { return <main><label htmlFor="item">First</label><label htmlFor="item">Second</label><input id="item" /></main>; }`,
			want:   "ambiguous control labels",
		},
		"dynamic label text": {
			source: `function View() { return <label>Name {suffix}<input /></label>; }`,
			want:   "exact literal label text",
		},
		"conditional control": {
			source: `function View() { return <main>{visible && <button type="button">Continue</button>}</main>; }`,
			want:   "dynamic control cardinality",
		},
		"explicit role": {
			source: `function View() { return <div role="button">Continue</div>; }`,
			want:   "unsupported attribute role",
		},
		"unused JSX local": {
			source: `function View() { const ghost = <button type="button">Ghost</button>; return <button type="button">Real</button>; }`,
			want:   "JSX outside the unconditional return",
		},
		"conditional returns": {
			source: `function View() { if (ready) { return <button type="button">Ready</button>; } return <button type="button">Wait</button>; }`,
			want:   "JSX outside the unconditional return",
		},
		"early null return": {
			source: `function View() { if (blocked) { return null; } return <button type="button">Run</button>; }`,
			want:   "return outside the unconditional top-level return",
		},
		"conditional throw": {
			source: `function View() { if (blocked) { throw new Error("blocked"); } return <button type="button">Run</button>; }`,
			want:   "throw in render function control flow",
		},
		"unregistered inventory event surface": {
			source: `function View() { return <div onClick={() => void 0}>Adjust stock</div>; }`,
			want:   "event attribute onClick on unregistered intrinsic element div",
		},
		"unregistered itinerary focus surface": {
			source: `function View() { return <div tabIndex="0">Choose itinerary</div>; }`,
			want:   "unsupported attribute tabIndex",
		},
		"runtime-mutated schedule surface": {
			source: `function View() { return <button type="button" ref={(node) => void node}>Publish schedule</button>; }`,
			want:   "unsupported attribute ref",
		},
		"multiple top-level returns": {
			source: `function View() { return <button type="button">First</button>; return <button type="button">Second</button>; }`,
			want:   "one unconditional top-level return",
		},
		"ternary render root": {
			source: `function View() { return ready ? <button type="button">Ready</button> : <button type="button">Wait</button>; }`,
			want:   "one unconditional intrinsic JSX root",
		},
		"closed dialog": {
			source: `function View() { return <dialog><button type="button">Continue</button></dialog>; }`,
			want:   "visibility-bearing dialog",
		},
		"embedded style": {
			source: `function View() { return <main><style>{"button { display: none }"}</style><button type="button">Continue</button></main>; }`,
			want:   `embedded non-public element "style"`,
		},
		"embedded script": {
			source: `function View() { return <main><script>{"void 0"}</script><button type="button">Continue</button></main>; }`,
			want:   `embedded non-public element "script"`,
		},
		"popover state": {
			source: `function View() { return <main popover><button type="button">Continue</button></main>; }`,
			want:   "unsupported attribute popover",
		},
		"conditional hidden class": {
			source: `function View() { return <main className="md:hidden"><button type="button">Continue</button></main>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"important hidden class": {
			source: `function View() { return <main className="!hidden"><button type="button">Continue</button></main>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"trailing important hidden class": {
			source: `function View() { return <main className="hidden!"><button type="button">Continue</button></main>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"arbitrary hidden class": {
			source: `function View() { return <main className="[display:none]"><button type="button">Continue</button></main>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"content visibility class": {
			source: `function View() { return <main className="[content-visibility:hidden]"><button type="button">Continue</button></main>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"transparent reservation control": {
			source: `function View() { return <button type="button" className="opacity-0">Reserve seat</button>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"pointer-disabled search control": {
			source: `function View() { return <button type="button" className="pointer-events-none">Run search</button>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"screen-reader-only checkout control": {
			source: `function View() { return <button type="button" className="sr-only">Complete checkout</button>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"zero-size clipped profile control": {
			source: `function View() { return <section className="w-0 overflow-hidden"><button type="button">Save profile</button></section>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"arbitrary clipped report control": {
			source: `function View() { return <section className="[clip-path:inset(50%)]"><button type="button">Export report</button></section>; }`,
			want:   "non-allowlisted Tailwind class",
		},
		"disabled fieldset billing control": {
			source: `function View() { return <fieldset disabled><button type="button">Submit payment</button></fieldset>; }`,
			want:   "disabled fieldset ancestry",
		},
		"svg visibility": {
			source: `function View() { return <main><svg display="none"><text>Hidden</text></svg></main>; }`,
			want:   `unsupported intrinsic element "svg"`,
		},
		"duplicate element id": {
			source: `function View() { return <main><input id="value" /><div id="value">Duplicate</div></main>; }`,
			want:   "repeats public element id",
		},
		"disabled control": {
			source: `function View() { return <button type="button" disabled>Continue</button>; }`,
			want:   "unavailable control state disabled",
		},
		"readonly control": {
			source: `function View() { return <input readOnly />; }`,
			want:   "unavailable control state readOnly",
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
