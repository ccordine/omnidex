package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGenericBrowserRuntimeSealsEverySharedValueBoundary(t *testing.T) {
	requirements := []assemblyline.Requirement{{}}
	source := genericBrowserRuntimeSource(requirements)
	for _, required := range []string{
		"const maximumSharedValueDepth = 64;",
		"const maximumSharedValueNodes = 10000;",
		"const frozenFallback = validateAndFreezeSharedValue(fallback, 'fallback for ' + capability);",
		"const frozenValue = validateAndFreezeSharedValue(value, 'publication for ' + capability);",
		"() => validateAndFreezeSharedValue(fallback, 'hook fallback for ' + capability)",
		"() => frozenFallback",
		"const emptyFeatureState: FeatureState = Object.freeze({});",
		"runtime.application.read(runtime.capability, emptyFeatureState)",
		"useOwnCapabilityState(runtime, emptyFeatureState)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("generated browser runtime omits SharedValue boundary %q", required)
		}
	}
	if strings.Contains(source, "() => fallback") {
		t.Fatal("generated browser runtime retains the unvalidated server fallback")
	}
}

func TestGenericBrowserRuntimeEmitsSharedValueBehaviorTests(t *testing.T) {
	source := genericBrowserRuntimeTestSource([]assemblyline.Requirement{{}})
	for _, required := range []string{
		"deep-freezes direct and server-rendered fallbacks",
		"accepts shared aliases and rejects unsupported publications atomically",
		"keeps action updates immutable",
		"expect(runtime.snapshot()).toBe(before)",
		"expect(capabilityChanges).toBe(0)",
		"expect(Object.isFrozen(validPrefix)).toBe(false)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("generated browser runtime tests omit behavior %q", required)
		}
	}
	if strings.Contains(source, "%!") {
		t.Fatalf("generated browser runtime tests contain an unresolved format directive: %s", source)
	}
}
