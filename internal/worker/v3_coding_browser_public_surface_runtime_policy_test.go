package worker

import (
	"strings"
	"testing"
)

func TestBrowserPublicSurfaceRejectsFreeHostRuntimeAuthority(t *testing.T) {
	for _, name := range []string{
		"document", "window", "globalThis", "self", "frames", "parent", "top", "opener",
		"navigator", "location", "history", "origin", "screen", "visualViewport", "customElements",
		"fetch", "XMLHttpRequest", "WebSocket", "EventSource", "WebTransport", "BroadcastChannel",
		"Worker", "SharedWorker", "ServiceWorker", "importScripts", "MessageChannel", "postMessage",
		"localStorage", "sessionStorage", "caches", "indexedDB", "cookieStore",
		"Audio", "Image", "AudioContext", "OfflineAudioContext",
		"webkitAudioContext", "webkitOfflineAudioContext",
		"DOMParser", "MutationObserver", "ResizeObserver", "IntersectionObserver", "FileReader",
		"URL", "Request", "Notification", "MediaRecorder", "RTCPeerConnection",
		"setTimeout", "setInterval", "queueMicrotask", "requestAnimationFrame", "requestIdleCallback",
		"alert", "confirm", "prompt", "open", "print", "matchMedia", "dispatchEvent",
		"crypto", "performance", "eval", "Function", "Proxy", "Reflect", "getSelection",
	} {
		t.Run(name, func(t *testing.T) {
			source := "function View() { void " + name + "; return <button type=\"button\">Run check</button>; }"
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			want := "runtime host authority identifier " + name
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want error containing %q, got %v", want, err)
			}
		})
	}
}

func TestBrowserPublicSurfaceRejectsEveryOtherFreeRuntimeIdentifier(t *testing.T) {
	source := `function View() {
  void ambientCapabilityNotDeclaredByTheAdapter;
  return <button type="button">Run check</button>;
}`
	_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
	if err == nil || !strings.Contains(
		err.Error(), "undeclared runtime identifier ambientCapabilityNotDeclaredByTheAdapter",
	) {
		t.Fatalf("want fail-closed free runtime rejection, got %v", err)
	}
}

func TestBrowserPublicSurfaceRejectsDynamicImportAndHostMetadata(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"dynamic import": {
			source: `function View() {
  const load = () => import("external-runtime");
  return <button type="button" onClick={() => void load()}>Load</button>;
}`,
			want: "dynamic import authority",
		},
		"host metadata": {
			source: `function View() {
  const location = import.meta.url;
  return <button type="button" onClick={() => void location}>Inspect</button>;
}`,
			want: "runtime host metadata authority",
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

func TestBrowserRuntimePolicyRejectsNondeterministicGlobals(t *testing.T) {
	fixtures := map[string]struct {
		source string
		want   string
	}{
		"random": {
			source: `function View() { return <button type="button" onClick={() => void Math.random()}>Refresh report</button>; }`,
			want:   "nondeterministic runtime property random",
		},
		"aliased random": {
			source: `function View() { const math = Math; return <button type="button" onClick={() => void math.random()}>Refresh report</button>; }`,
			want:   "runtime global value escape Math",
		},
		"destructured random": {
			source: `function View() { const { random } = Math; return <button type="button" onClick={() => void random()}>Refresh report</button>; }`,
			want:   "nondeterministic runtime property random",
		},
		"host locale": {
			source: `function View() { return <button type="button" onClick={() => void new Intl.NumberFormat().format(1000)}>Format report</button>; }`,
			want:   "runtime host authority identifier Intl",
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

func TestBrowserRuntimePolicyRejectsEscapedPropertyAuthority(t *testing.T) {
	fixtures := map[string]string{
		"random": `function View() {
  return <button type="button" onClick={() => void Math.r\u0061ndom()}>Refresh report</button>;
}`,
		"locale": `function View() {
  const value = Number(1000);
  return <button type="button" onClick={() => void value.toLocaleStr\u0069ng()}>Format report</button>;
}`,
		"reflection": `function View() {
  const escape = ({}).constr\u0075ctor.constr\u0075ctor("return globalThis")();
  return <button type="button" onClick={() => void escape}>Inspect</button>;
}`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "escaped runtime identifier") {
				t.Fatalf("want escaped runtime identifier rejection, got %v", err)
			}
		})
	}
}

func TestBrowserRuntimePolicyRejectsHostLocalePrototypeMethods(t *testing.T) {
	for _, method := range []string{
		"localeCompare", "toLocaleLowerCase", "toLocaleUpperCase",
		"toLocaleString", "toLocaleDateString", "toLocaleTimeString",
	} {
		t.Run(method, func(t *testing.T) {
			source := `function View() {
  const value = Number(1000);
  return <button type="button" onClick={() => void value.` + method + `()}>Format report</button>;
}`
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			want := "nondeterministic runtime property " + method
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v, want %q", err, want)
			}
			destructured := `function View() {
  const value = Number(1000);
  const { ` + method + ` } = value;
  return <button type="button" onClick={() => void ` + method + `()}>Format report</button>;
}`
			_, err = extractDirectCodingBrowserPublicInteractionSurface(destructured)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("destructured error=%v, want %q", err, want)
			}
		})
	}
}

func TestBrowserRuntimePolicyRejectsErrorHostAuthorityProperties(t *testing.T) {
	fixtures := map[string]struct {
		source   string
		property string
	}{
		"instance stack": {
			source: `function View() {
  return <button type="button" onClick={() => void new Error("failure").stack}>Inspect failure</button>;
}`,
			property: "stack",
		},
		"capture stack trace": {
			source: `function View() {
  const target = {};
  return <button type="button" onClick={() => void Error.captureStackTrace(target)}>Capture failure</button>;
}`,
			property: "captureStackTrace",
		},
		"prepare stack trace": {
			source: `function View() {
  Error.prepareStackTrace = () => "host detail";
  return <button type="button">Prepare failure</button>;
}`,
			property: "prepareStackTrace",
		},
		"stack trace limit": {
			source: `function View() {
  return <button type="button" onClick={() => void Error.stackTraceLimit}>Inspect limit</button>;
}`,
			property: "stackTraceLimit",
		},
		"destructured stack": {
			source: `function View() {
  const { stack } = new Error("failure");
  return <button type="button" onClick={() => void stack}>Inspect failure</button>;
}`,
			property: "stack",
		},
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(fixture.source)
			want := "nondeterministic runtime property " + fixture.property
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v, want %q", err, want)
			}
		})
	}
}

func TestBrowserRuntimePolicyAllowsLocalErrorPropertyNames(t *testing.T) {
	source := `function View() {
  const diagnostics = {
    stack: "local stack",
    captureStackTrace: () => "local capture",
    prepareStackTrace: () => "local prepare",
    stackTraceLimit: 4,
  };
  const { stack, captureStackTrace } = diagnostics;
  return <main>
    <button type="button" onClick={() => void stack}>Local stack label</button>
    <button type="button" onClick={() => void captureStackTrace()}>Local capture label</button>
    <button type="button" onClick={() => void diagnostics.prepareStackTrace()}>Local prepare label</button>
    <button type="button" onClick={() => void diagnostics.stackTraceLimit}>Local limit label</button>
  </main>;
}`
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(source); err != nil {
		t.Fatalf("locally owned Error property names were rejected: %v", err)
	}
}

func TestBrowserRuntimePolicyAllowsLocallyOwnedMethodNames(t *testing.T) {
	source := `function View() {
	const formatter = {
    random: () => 0,
		localeCompare: () => 0,
		toLocaleLowerCase: () => "local lower",
		toLocaleUpperCase: () => "local upper",
    toLocaleString: () => "local number",
    toLocaleDateString: () => "local date",
    toLocaleTimeString: () => "local time",
  };
	const { toLocaleString } = formatter;
  return <main>
    <button type="button" onClick={() => void formatter.random()}>Local random label</button>
		<button type="button" onClick={() => void formatter.localeCompare()}>Local compare label</button>
		<button type="button" onClick={() => void formatter.toLocaleLowerCase()}>Local lower label</button>
		<button type="button" onClick={() => void formatter.toLocaleUpperCase()}>Local upper label</button>
    <button type="button" onClick={() => void toLocaleString()}>Local number label</button>
    <button type="button" onClick={() => void formatter.toLocaleDateString()}>Local date label</button>
    <button type="button" onClick={() => void formatter.toLocaleTimeString()}>Local time label</button>
  </main>;
}`
	if _, err := extractDirectCodingBrowserPublicInteractionSurface(source); err != nil {
		t.Fatalf("locally owned method names were rejected: %v", err)
	}
}

func TestBrowserRuntimePolicyRejectsUnstableLocalMethodExceptions(t *testing.T) {
	fixtures := map[string]string{
		"spread override": `function View({ capabilities }) {
  const formatter = { random: () => 0, ...capabilities };
  return <button type="button" onClick={() => void formatter.random()}>Local random label</button>;
}`,
		"property reassignment": `function View({ capabilities }) {
  const formatter = { random: () => 0 };
  formatter.random = capabilities.pick;
  return <button type="button" onClick={() => void formatter.random()}>Local random label</button>;
}`,
		"object assign": `function View({ capabilities }) {
  const formatter = { random: () => 0 };
  Object.assign(formatter, capabilities);
  return <button type="button" onClick={() => void formatter.random()}>Local random label</button>;
}`,
		"computed object assign": `function View({ capabilities }) {
  const formatter = { random: () => 0 };
  Object["assign"](formatter, capabilities);
  return <button type="button" onClick={() => void formatter.random()}>Local random label</button>;
}`,
	}
	for name, source := range fixtures {
		t.Run(name, func(t *testing.T) {
			_, err := extractDirectCodingBrowserPublicInteractionSurface(source)
			if err == nil || !strings.Contains(err.Error(), "nondeterministic runtime property random") {
				t.Fatalf("want unstable local method rejection, got %v", err)
			}
		})
	}
}
