package worker

import (
	"strings"
	"testing"
)

func TestBrowserPublicOutputRejectsStaticAndUnprovenAliases(t *testing.T) {
	fixtures := map[string]string{
		"constant alias": `function View() {
  const result = 42;
  return <output aria-label="Inventory total">{result}</output>;
}`,
		"constant alias chain": `function View() {
  const base = "ready";
  const result = String(base);
  return <output aria-label="Report state">{result}</output>;
}`,
		"constant object": `function View() {
  const result = { value: 42 };
  return <output aria-label="Travel estimate">{result.value}</output>;
}`,
		"static memo": `function View() {
  const result = useMemo(() => 42, []);
  return <output aria-label="Report total">{result}</output>;
}`,
		"free identifier": `function View() {
  return <output aria-label="Inventory total">{result}</output>;
}`,
		"constant setter": `function View() {
  const [result, setResult] = useState("x");
  return <main><button type="button" onClick={() => setResult("ALPHA")}>Refresh report</button><output aria-label="Report state">{result}</output></main>;
}`,
		"constant setter alias": `function View() {
  const [result, setResult] = useState("x");
  const next = "ALPHA";
  return <main><button type="button" onClick={() => setResult(next)}>Refresh report</button><output aria-label="Report state">{result}</output></main>;
}`,
		"unrelated derived setter": `function View() {
  const [query, setQuery] = useState("");
  const [result, setResult] = useState("x");
  return <main><input aria-label="Search inventory" value={query} onChange={(event) => setQuery(event.target.value)} /><button type="button" onClick={() => setResult("ALPHA")}>Run search</button><output aria-label="Search result">{result}</output></main>;
}`,
		"discarded state sequence": `function View({ state }) {
  return <output aria-label="Report state">{(state.report, "ALPHA")}</output>;
}`,
		"annihilated state value": `function View({ state }) {
  return <output aria-label="Report state">{void /* discard */ state.report}</output>;
}`,
		"discarded event setter value": `function View() {
  const [result, setResult] = useState("");
  return <main><input aria-label="Report input" onChange={(event) => setResult((event.target.value, "ALPHA"))} /><output aria-label="Report state">{result}</output></main>;
}`,
		"annihilated event setter value": `function View() {
  const [result, setResult] = useState("");
  return <main><input aria-label="Report input" onChange={(event) => setResult(void event.target.value)} /><output aria-label="Report state">{result}</output></main>;
}`,
		"out of scope named event handler": `function View() {
  const [result, setResult] = useState("");
  { function handleChange(event) { setResult(event.target.value); } }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"constant selected object property": `function View({ state }) {
  const result = { decoy: state.report, value: "ALPHA" };
  return <output aria-label="Report state">{result.value}</output>;
}`,
		"constant selected array element": `function View({ state }) {
  const result = [state.report, "ALPHA"];
  return <output aria-label="Report state">{result[1]}</output>;
}`,
		"constant destructured object property": `function View({ state }) {
  const source = { decoy: state.report, value: "ALPHA" };
  const { value } = source;
  return <output aria-label="Report state">{value}</output>;
}`,
		"constant destructured array element": `function View({ state }) {
  const source = ["ALPHA", state.report];
  const [value] = source;
  return <output aria-label="Report state">{value}</output>;
}`,
		"constant assignment result": `function View({ state }) {
  return <output aria-label="Report state">{state.report = "ALPHA"}</output>;
}`,
		"named handler constant selected property": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) {
    setResult({ decoy: event.target.value, value: "ALPHA" }.value);
  }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"named handler constant assignment result": `function View() {
  const [result, setResult] = useState("");
  const source = { value: "" };
  function handleChange(event) { setResult(source.value = "ALPHA"); void event.target.value; }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"ignored local function argument": `function View({ state }) {
  function format(value) { return "ALPHA"; }
  return <output aria-label="Report state">{format(state.report)}</output>;
}`,
		"decoy in local function return": `function View({ state }) {
  function format() { return { decoy: state.report, value: "ALPHA" }.value; }
  return <output aria-label="Report state">{format()}</output>;
}`,
		"selected constant from local function object": `function View({ state }) {
  function format(value) { return { decoy: value, result: "ALPHA" }; }
  return <output aria-label="Report state">{format(state.report).result}</output>;
}`,
		"ignored local object method argument": `function View({ state }) {
  const formatter = { format(value) { return "ALPHA"; } };
  return <output aria-label="Report state">{formatter.format(state.report)}</output>;
}`,
		"static memo with derived dependency": `function View({ state }) {
  const result = useMemo(() => "ALPHA", [state.report]);
  return <output aria-label="Report state">{result}</output>;
}`,
		"ignored builtin argument": `function View({ state }) {
  return <output aria-label="Report state">{String("ALPHA", state.report)}</output>;
}`,
		"ignored instance builtin argument": `function View({ state }) {
  function constant(value) { return value; }
  return <output aria-label="Report state">{constant("ALPHA").trim(state.report)}</output>;
}`,
		"named handler ignored local method argument": `function View() {
  const [result, setResult] = useState("");
  const formatter = { format(value) { return "ALPHA"; } };
  function handleChange(event) { setResult(formatter.format(event.target.value)); }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter in false if arm": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { if (false) { setResult(event.target.value); } }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter in true if else arm": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { if (true) { void event.target.value; } else { setResult(event.target.value); } }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter after false logical and": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { false && setResult(event.target.value); }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter after true logical or": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { true || setResult(event.target.value); }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter in false ternary arm": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { false ? setResult(event.target.value) : void event.target.value; }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter in false while body": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { while (false) { setResult(event.target.value); } }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter after return": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { return; setResult(event.target.value); }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter after throw": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { throw new Error("stop"); setResult(event.target.value); }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "not runtime-derived") {
				t.Fatalf("error=%v, want unproven output dataflow rejection", err)
			}
		})
	}
}

func TestBrowserPublicOutputAcceptsInteractionAndRuntimeDataflow(t *testing.T) {
	fixtures := map[string]string{
		"event value": `function View() {
  const [result, setResult] = useState("");
  return <main><input aria-label="Inventory code" value={result} onChange={(event) => setResult(event.target.value)} /><output aria-label="Selected inventory code">{result}</output></main>;
}`,
		"event checked": `function View() {
  const [result, setResult] = useState(false);
  return <main><input type="checkbox" aria-label="Include archived" checked={result} onChange={(event) => setResult(event.currentTarget.checked)} /><output aria-label="Archive selection">{String(result)}</output></main>;
}`,
		"derived local states": `function View() {
  const [departure, setDeparture] = useState("");
  const [arrival, setArrival] = useState("");
  const [route, setRoute] = useState("");
  return <main><input aria-label="Departure city" value={departure} onChange={(event) => setDeparture(event.target.value)} /><input aria-label="Arrival city" value={arrival} onChange={(event) => setArrival(event.target.value)} /><button type="button" onClick={() => setRoute(departure + arrival)}>Find route</button><output aria-label="Route summary">{route}</output></main>;
}`,
		"functional updater": `function View() {
  const [count, setCount] = useState(0);
  return <main><button type="button" onClick={() => setCount((previous) => previous + 1)}>Increment count</button><output aria-label="Current count">{count}</output></main>;
}`,
		"named event handler": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { setResult(event.target.value); }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"declared state": `function View({ state }) {
  const result = String(state.total);
  return <output aria-label="Inventory total">{result}</output>;
}`,
		"declared capability": `function View({ capabilities }) {
  const result = capabilities.routeSummary;
  return <output aria-label="Route summary">{result}</output>;
}`,
		"selected derived object property": `function View({ state }) {
  const result = { value: state.report, decoy: "ALPHA" };
  return <output aria-label="Report state">{result.value}</output>;
}`,
		"selected derived array element": `function View({ state }) {
  const result = ["ALPHA", state.report];
  return <output aria-label="Report state">{result[1]}</output>;
}`,
		"destructured derived object property": `function View({ state }) {
  const source = { value: state.report, decoy: "ALPHA" };
  const { value } = source;
  return <output aria-label="Report state">{value}</output>;
}`,
		"destructured derived array element": `function View({ state }) {
  const source = [state.report, "ALPHA"];
  const [value] = source;
  return <output aria-label="Report state">{value}</output>;
}`,
		"local function parameter": `function View({ state }) {
  function format(value) { return value + "!"; }
  return <output aria-label="Report state">{format(state.report)}</output>;
}`,
		"selected local function parameter": `function View({ state }) {
  function format(value) { return { result: value, decoy: "ALPHA" }; }
  return <output aria-label="Report state">{format(state.report).result}</output>;
}`,
		"local object method parameter": `function View({ state }) {
  const formatter = { format(value) { return value + "!"; } };
  return <output aria-label="Report state">{formatter.format(state.report)}</output>;
}`,
		"derived memo value": `function View({ state }) {
  const result = useMemo(() => String(state.report), [state.report]);
  return <output aria-label="Report state">{result}</output>;
}`,
		"value producing builtins": `function View({ state }) {
  const result = String(Math.max(Number(state.report), 0));
  return <output aria-label="Report state">{result}</output>;
}`,
		"named handler local transform": `function View() {
  const [source, setSource] = useState("");
  const [result, setResult] = useState("");
  function normalize(value) { return value.trim().toUpperCase(); }
  function handleChange(event) {
    setSource(event.target.value);
    setResult(normalize(event.target.value));
  }
  return <main><input aria-label="Report input" value={source} onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter in true if arm": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { if (true) { setResult(event.target.value); } }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter in false if else arm": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { if (false) { void event.target.value; } else { setResult(event.target.value); } }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter in runtime dependent arm": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { if (event.target.value) { setResult(event.target.value); } }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
		"setter before return": `function View() {
  const [result, setResult] = useState("");
  function handleChange(event) { setResult(event.target.value); return; }
  return <main><input aria-label="Report input" onChange={handleChange} /><output aria-label="Report state">{result}</output></main>;
}`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			surface, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err != nil {
				t.Fatalf("runtime-owned output dataflow was rejected: %v", err)
			}
			if len(surface.Outputs) != 1 {
				t.Fatalf("output count=%d, want 1", len(surface.Outputs))
			}
		})
	}
}
