package worker

import (
	"strings"
	"testing"
)

func TestBrowserRuntimePolicyRejectsPermittedGlobalMutation(t *testing.T) {
	fixtures := map[string]string{
		"property assignment":   `Object.is = () => false;`,
		"augmented assignment":  `Math.PI += 1;`,
		"property update":       `Number.MAX_VALUE++;`,
		"property delete":       `delete JSON.stringify;`,
		"identifier assignment": `String = () => "changed";`,
		"nested property":       `Object.is.extra = true;`,
		"array target":          `[Object.is] = [() => false];`,
		"object target":         `({ x: Object.is } = { x: () => false });`,
		"for of target":         `for (Object.is of [() => false]) { void Object.is; }`,
	}
	for name, operation := range fixtures {
		t.Run(name, func(t *testing.T) {
			source := `function View() { ` + operation + ` return <button type="button">Inspect runtime</button>; }`
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "runtime global mutation") {
				t.Fatalf("want runtime global mutation rejection, got %v", err)
			}
		})
	}
}

func TestBrowserRuntimePolicyRejectsPermittedGlobalEscapeAndIndirectMutation(t *testing.T) {
	fixtures := map[string]string{
		"simple alias":       `const GlobalObject = Object; GlobalObject.is = () => false;`,
		"holder alias":       `const holder = { global: Object }; holder.global.is = () => false;`,
		"local mutator":      `const poison = (global: { is: unknown }) => { global.is = false; }; poison(Object);`,
		"assign mutator":     `Object.assign(Object, { is: () => false });`,
		"freeze mutator":     `Object.freeze(Object);`,
		"seal mutator":       `Object.seal(Object);`,
		"prevent extensions": `Object.preventExtensions(Object);`,
		"property alias":     `const keys = Object.keys; (keys as { changed?: boolean }).changed = true;`,
		"property argument":  `const poison = (value: object) => { Object.freeze(value); }; poison(Object.keys);`,
		"value of alias":     `const GlobalObject = Object.valueOf(); GlobalObject.is = () => false;`,
	}
	for name, operation := range fixtures {
		t.Run(name, func(t *testing.T) {
			source := `function View() { ` + operation + ` return <button type="button">Inspect runtime</button>; }`
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || (!strings.Contains(err.Error(), "runtime global value escape") &&
				!strings.Contains(err.Error(), "runtime global property value escape")) {
				t.Fatalf("want runtime global escape rejection, got %v", err)
			}
		})
	}
}

func TestBrowserRuntimePolicyAllowsDirectBuiltinUseAndLocalMutation(t *testing.T) {
	fixtures := map[string]string{
		"direct calls": `function View() {
  const maximum = Math.max(Number("2"), parseInt("3", 10));
  const keys = Object.keys({ alpha: true });
  const encoded = JSON.stringify(keys);
  const expression = new RegExp("alpha", "i");
  return <button type="button" onClick={() => void [maximum, encoded, expression.test("ALPHA")]}>Inspect values</button>;
}`,
		"stable constants": `function View() {
  const circumference = 2 * Math.PI * Number.EPSILON;
  const iterator = Symbol.iterator;
  return <button type="button" onClick={() => void [circumference, iterator]}>Inspect constants</button>;
}`,
		"local shadows": `function View() {
  const Object = { is: true };
  let String = { value: "local" };
  const Math = { PI: 3 };
  Object.is = false;
  Math.PI += 1;
  String = { value: "changed" };
  delete String.value;
  return <button type="button" onClick={() => void [Object.is, Math.PI, String]}>Inspect local</button>;
}`,
		"local results": `function View() {
  const value = Object({ alpha: true });
  const keys = Object.keys(value);
  value.extra = "local";
  keys.push("extra");
  Object.freeze(value);
  return <button type="button" onClick={() => void [value.extra, keys.length]}>Inspect local</button>;
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

func TestBrowserRuntimePolicyRejectsLegacyReflectionProperties(t *testing.T) {
	for _, property := range []string{
		"__defineGetter__", "__defineSetter__", "__lookupGetter__", "__lookupSetter__",
		"call", "apply", "bind", "hasOwnProperty", "propertyIsEnumerable",
	} {
		t.Run(property, func(t *testing.T) {
			source := `function View() {
  const local = { ` + property + `: () => undefined };
  return <button type="button" onClick={() => void local["` + property + `"]()}>Inspect reflection</button>;
}`
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			want := "runtime reflection property " + property
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v, want %q", err, want)
			}
		})
	}

	destructured := `function View() {
  const { ["__lookup" + "Getter__"]: lookup } = {};
  return <button type="button" onClick={() => void lookup}>Inspect reflection</button>;
}`
	_, err := extractDirectCodingBrowserPublicInteractionSurface(destructured)
	if err == nil || !strings.Contains(err.Error(), "runtime reflection property __lookupGetter__") {
		t.Fatalf("want destructured reflection rejection, got %v", err)
	}
}

func TestBrowserRuntimePolicyRejectsVerifierPrototypeMethodPoisoning(t *testing.T) {
	fixtures := map[string]string{
		"has own direct":    `({} as any).hasOwnProperty.call = () => true;`,
		"enumerable direct": `({} as any).propertyIsEnumerable.call = () => true;`,
		"alias":             `const method = ({} as any).hasOwnProperty; method.call = () => true;`,
		"local helper":      `const poison = (method: any) => { method.call = () => true; }; poison(({} as any).hasOwnProperty);`,
		"assign target":     `Object.assign(({} as any).hasOwnProperty, { call: () => true });`,
	}
	for name, operation := range fixtures {
		t.Run(name, func(t *testing.T) {
			source := `function View() { ` + operation + ` return <button type="button">Inspect runtime</button>; }`
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "runtime reflection property") {
				t.Fatalf("want verifier prototype-method rejection, got %v", err)
			}
		})
	}
}
