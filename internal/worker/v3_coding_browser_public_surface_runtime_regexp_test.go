package worker

import (
	"strconv"
	"strings"
	"testing"
)

func TestBrowserRuntimePolicyRejectsRegExpRealmGlobalProperties(t *testing.T) {
	properties := []string{
		"input", "$_", "lastMatch", "$&", "lastParen", "$+",
		"leftContext", "$`", "rightContext", "$'",
		"$1", "$2", "$3", "$4", "$5", "$6", "$7", "$8", "$9",
	}
	for _, property := range properties {
		t.Run(property, func(t *testing.T) {
			source := `function View() {
  return <button type="button" onClick={() => void RegExp[` + strconv.Quote(property) + `]}>Inspect match</button>;
}`
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			want := "RegExp realm-global property " + property
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v, want %q", err, want)
			}
		})
	}
}

func TestBrowserRuntimePolicyRejectsRegExpRealmGlobalAccessForms(t *testing.T) {
	fixtures := map[string]struct {
		source string
		want   string
	}{
		"direct read": {
			source: `function View() {
  return <button type="button" onClick={() => void RegExp.input}>Inspect match</button>;
}`,
			want: "RegExp realm-global property input",
		},
		"computed read": {
			source: `function View() {
  return <button type="button" onClick={() => void RegExp["in" + "put"]}>Inspect match</button>;
}`,
			want: "RegExp realm-global property input",
		},
		"write": {
			source: `function View() {
  RegExp.input = "mutated";
  return <button type="button">Inspect match</button>;
}`,
			want: "RegExp realm-global property input",
		},
		"destructure": {
			source: `function View() {
  const { lastMatch } = RegExp;
  return <button type="button" onClick={() => void lastMatch}>Inspect match</button>;
}`,
			want: "RegExp realm-global property lastMatch",
		},
		"renamed destructure": {
			source: `function View() {
  const { ["$" + "&"]: match } = RegExp;
  return <button type="button" onClick={() => void match}>Inspect match</button>;
}`,
			want: "RegExp realm-global property $&",
		},
		"simple alias": {
			source: `function View() {
  const RuntimeRegExp = RegExp;
  return <button type="button" onClick={() => void RuntimeRegExp.input}>Inspect match</button>;
}`,
			want: "runtime global value escape RegExp",
		},
		"holder alias": {
			source: `function View() {
  const holder = { runtime: RegExp };
  return <button type="button" onClick={() => void holder.runtime.input}>Inspect match</button>;
}`,
			want: "runtime global value escape RegExp",
		},
		"indirect reader": {
			source: `function View() {
  const read = (runtime: { input: string }) => runtime.input;
  return <button type="button" onClick={() => void read(RegExp)}>Inspect match</button>;
}`,
			want: "runtime global value escape RegExp",
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

func TestBrowserRuntimePolicyAllowsLocalRegExpPropertyNamesAndInstances(t *testing.T) {
	fixtures := map[string]string{
		"instance operations": `function View() {
  const expression = new RegExp("a", "g");
  const callable = RegExp("b");
  return <button type="button" onClick={() => {
    void expression.test("a");
    void expression.source;
    void expression.lastIndex;
    void callable.exec("b");
  }}>Inspect expressions</button>;
}`,
		"local owned names": `function View() {
  const local = { input: "local", lastMatch: "match", "$1": "capture", "$&": "whole" };
  const { input, lastMatch } = local;
  return <button type="button" onClick={() => void [input, lastMatch, local["$1"], local["$&"]]}>Inspect local</button>;
}`,
		"shadowed RegExp": `function View() {
  const RegExp = { input: "local", lastMatch: "match" };
  return <button type="button" onClick={() => void [RegExp.input, RegExp.lastMatch]}>Inspect local</button>;
}`,
		"domain input": `function View({ state }: { state: { input: string } }) {
  return <button type="button" onClick={() => void state.input}>Inspect input</button>;
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
