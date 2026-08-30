package worker

import (
	"strings"
	"testing"
)

func TestBrowserRuntimePolicyRejectsAuthoritativeStateMutation(t *testing.T) {
	fixtures := map[string]string{
		"direct state property": `function View({ state }) {
  (state as any).report = "ALPHA";
  return <output aria-label="Report">{state.report}</output>;
}`,
		"direct capability property": `function View({ capabilities }) {
  (capabilities as any).summary = "ALPHA";
  return <output aria-label="Summary">{capabilities.summary}</output>;
}`,
		"nested property": `function View({ state }) {
  (state.profile as any).name = "ALPHA";
  return <output aria-label="Name">{state.profile.name}</output>;
}`,
		"member alias": `function View({ state }) {
  const profile = state.profile;
  (profile as any).name = "ALPHA";
  return <output aria-label="Name">{state.profile.name}</output>;
}`,
		"destructured alias": `function View({ capabilities }) {
  const { summary } = capabilities;
  (summary as any).value = "ALPHA";
  return <output aria-label="Summary">{capabilities.summary}</output>;
}`,
		"root rebind": `function View({ state }) {
  state = { report: "ALPHA" };
  return <output aria-label="Report">{state.report}</output>;
}`,
		"alias rebind": `function View({ state }) {
  let result = state;
  result = { report: "ALPHA" };
  return <output aria-label="Report">{result.report}</output>;
}`,
		"delete nested": `function View({ state }) {
  delete (state as any).report;
  return <output aria-label="Report">{state.report}</output>;
}`,
		"update nested": `function View({ state }) {
  (state as any).count++;
  return <output aria-label="Count">{state.count}</output>;
}`,
		"destructuring target": `function View({ state }) {
  [state.report] = ["ALPHA"];
  return <output aria-label="Report">{state.report}</output>;
}`,
		"for of target": `function View({ state }) {
  for (state.report of ["ALPHA"]) { void state.report; }
  return <output aria-label="Report">{state.report}</output>;
}`,
		"array mutator": `function View({ state }) {
  (state.items as any[]).push("ALPHA");
  return <output aria-label="Items">{String(state.items)}</output>;
}`,
		"object assign": `function View({ capabilities }) {
  Object.assign(capabilities, { summary: "ALPHA" });
  return <output aria-label="Summary">{capabilities.summary}</output>;
}`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "authoritative state mutation") {
				t.Fatalf("want authoritative state mutation rejection, got %v", err)
			}
		})
	}
}

func TestBrowserRuntimePolicyAllowsImmutableStateUseAndActions(t *testing.T) {
	fixtures := map[string]string{
		"action update": `function View({ state, actions }) {
  return <main><button type="button" onClick={() => actions.set("report", "ALPHA")}>Update</button><output aria-label="Report">{state.report}</output></main>;
}`,
		"immutable copy": `function View({ state, actions }) {
  const next = { ...state, report: "ALPHA" };
  return <main><button type="button" onClick={() => actions.set("draft", next)}>Stage</button><output aria-label="Report">{state.report}</output></main>;
}`,
		"local mutation": `function View({ state }) {
  const draft = { report: String(state.report) };
  draft.report = draft.report.trim();
  return <output aria-label="Report">{state.report}</output>;
}`,
		"object copy": `function View({ state }) {
  const copy = Object.assign({}, state);
  copy.local = "ALPHA";
  return <output aria-label="Report">{state.report}</output>;
}`,
		"derived arithmetic": `function View({ state }) {
  let total = Number(state.total) + 1;
  total++;
  return <output aria-label="Total">{String(state.total)}</output>;
}`,
		"shadowed nested parameter": `function View({ state }) {
  function prepare(state: { value: string }) { state.value = state.value.trim(); return state; }
  const local = prepare({ value: "ALPHA" });
  return <output aria-label="Report">{String(state.report) + local.value}</output>;
}`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			if _, err := extractDirectCodingBrowserPublicInteractionSurface(source); err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
		})
	}
}
