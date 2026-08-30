package worker

import (
	"strings"
	"testing"
)

func TestBrowserPublicSurfaceRejectsUnresolvedComputedPropertyAuthority(t *testing.T) {
	tests := map[string]string{
		"local constructor key": `function View() {
  const key = "constructor";
  const escape = ({})[key];
  return <button type="button" onClick={() => void escape}>Inspect</button>;
}`,
		"local prototype chain keys": `function View() {
  const prototypeKey = "prototype";
  const constructorKey = "constructor";
  const escape = ({})[prototypeKey][constructorKey];
  return <button type="button" onClick={() => void escape}>Inspect</button>;
}`,
		"computed parameter key": `function View() {
  const read = (key: string) => ({})[key];
  return <button type="button" onClick={() => void read("constructor")}>Inspect</button>;
}`,
		"mutable numeric key": `function View() {
  let index = 1;
  const values = ["first", "second"];
  return <button type="button" onClick={() => void values[index]}>Inspect</button>;
}`,
		"shadowed numeric key": `function View() {
  const index = 1;
  const read = (index: string) => ({})[index];
  return <button type="button" onClick={() => void read("constructor")}>Inspect</button>;
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "unresolved computed property authority") {
				t.Fatalf("want unresolved computed property rejection, got %v", err)
			}
		})
	}
}

func TestBrowserPublicSurfaceRejectsReflectiveDestructuring(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"static constructor": {
			source: `function View() {
  const { constructor: HostFunction } = (() => {});
  return <button type="button" onClick={() => void HostFunction}>Inspect</button>;
}`,
			want: "runtime reflection property constructor",
		},
		"shorthand constructor": {
			source: `function View() {
  const { constructor } = (() => {});
  return <button type="button" onClick={() => void constructor}>Inspect</button>;
}`,
			want: "runtime reflection property constructor",
		},
		"dynamic constructor": {
			source: `function View() {
  const key = "constructor";
  const { [key]: HostFunction } = (() => {});
  return <button type="button" onClick={() => void HostFunction}>Inspect</button>;
}`,
			want: "unresolved computed destructured property authority",
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

func TestBrowserPublicSurfaceAllowsExactNumericDomainIndices(t *testing.T) {
	tests := map[string]string{
		"literal": `function View() {
  const values = ["first", "second"];
  return <button type="button" onClick={() => void values[1]}>Inspect</button>;
}`,
		"immutable local": `function View() {
  const values = ["first", "second"];
  const index = 1;
  return <button type="button" onClick={() => void values[index]}>Inspect</button>;
}`,
		"static property": `function View() {
  const value = { label: "ready" };
  return <button type="button" onClick={() => void value["label"]}>Inspect</button>;
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := extractDirectCodingBrowserPublicInteractionSurface(source); err != nil {
				t.Fatalf("extract safe indexed domain data: %v", err)
			}
		})
	}
}
