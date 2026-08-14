package worker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGenericBrowserAdapterCreatesOneCapabilityAndBlindAcceptancePerRequirement(t *testing.T) {
	t.Parallel()

	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "compact catalog browser",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "filter the catalog"},
			{ID: "requirement_002", SourceQuote: "remember my selection"},
		},
	}
	adapter, blueprint, staticFiles, err := compileGenericTypeScriptBrowserBlueprint(
		"unseen-app", specification, genericBrowserSkillBindings(specification),
		genericBrowserWorkload(t, specification),
		genericBrowserCapabilityBindings(specification),
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter != genericTypeScriptBrowserAdapter || len(staticFiles) == 0 {
		t.Fatalf("adapter=%q static_files=%d", adapter, len(staticFiles))
	}

	generated := make([]assemblyline.TypeScriptBlock, 0)
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.Generated() {
				generated = append(generated, block)
			}
		}
	}
	if len(generated) != len(specification.Requirements)*2 {
		t.Fatalf("generated blocks=%d want=%d", len(generated), len(specification.Requirements)*2)
	}
	if !strings.Contains(generated[0].Contract, "Exact user requirement: "+specification.Requirements[0].SourceQuote) ||
		strings.Contains(generated[0].Contract, "Exact user requirement: "+specification.Requirements[1].SourceQuote) {
		t.Fatalf("first feature received the wrong implementation authority:\n%s", generated[0].Contract)
	}
	if !strings.Contains(generated[1].Contract, "Exact user requirement: "+specification.Requirements[1].SourceQuote) ||
		strings.Contains(generated[1].Contract, "Exact user requirement: "+specification.Requirements[0].SourceQuote) {
		t.Fatalf("second feature received the wrong implementation authority:\n%s", generated[1].Contract)
	}
	for _, block := range generated {
		for _, required := range []string{
			string(specification.Surface), specification.ProductQuote,
		} {
			if !strings.Contains(block.Contract, required) {
				t.Fatalf("worker omitted executable-job authority %q:\n%s", required, block.Contract)
			}
		}
	}
}

func TestGenericBrowserAdapterContainsNoHeldOutProductImplementation(t *testing.T) {
	t.Parallel()

	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "unknown browser product",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "show the result",
		}},
	}
	_, blueprint, staticFiles, err := compileGenericTypeScriptBrowserBlueprint(
		"unknown", specification, genericBrowserSkillBindings(specification),
		genericBrowserWorkload(t, specification),
		genericBrowserCapabilityBindings(specification),
	)
	if err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	for _, file := range staticFiles {
		source.WriteString(file.Content)
	}
	for _, document := range blueprint.Documents {
		source.WriteString(document.Header)
		for _, block := range document.Blocks {
			source.WriteString(block.Static)
			if !block.Generated() {
				source.WriteString(block.Contract)
			}
		}
	}
	for _, forbidden := range []string{
		"audio workstation", "sequencer", "drum pad", "piano", "expense", "habit", "unit converter",
	} {
		if strings.Contains(strings.ToLower(source.String()), forbidden) {
			t.Fatalf("generic adapter embeds held-out product %q", forbidden)
		}
	}
}

func TestGenericBrowserAdapterOwnsCapabilityChannelsAndAcceptanceFailureRouting(t *testing.T) {
	t.Parallel()

	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "interactive studio",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "select a source"},
			{ID: "requirement_002", SourceQuote: "trigger the source"},
		},
	}
	_, blueprint, _, err := compileGenericTypeScriptBrowserBlueprint(
		"linked", specification, genericBrowserSkillBindings(specification),
		genericBrowserWorkload(t, specification),
		genericBrowserCapabilityBindings(specification),
	)
	if err != nil {
		t.Fatal(err)
	}
	for index, blockID := range []string{"feature.001", "feature.002"} {
		block, exists := directCodingTypeScriptBlueprintBlock(blueprint, blockID)
		if !exists {
			t.Fatalf("missing %s", blockID)
		}
		if !strings.Contains(block.Signature, "View({ state, capabilities, actions }: Feature") ||
			!strings.Contains(block.Signature, "ViewProps)") ||
			!strings.Contains(block.Contract, "Validated procedure:") ||
			!strings.Contains(block.Contract, "standard browser APIs are available") ||
			len(block.Policy.RequiredCalls) != 0 {
			t.Fatalf("view block escaped code-owned controller ownership: %#v", block)
		}
		for _, hook := range []string{"useEffect", "useRef", "useState"} {
			if !containsString(block.Globals, hook) {
				t.Fatalf("feature %d cannot implement its own workload behavior with %s", index+1, hook)
			}
		}
		wrapper, exists := directCodingTypeScriptBlueprintBlock(blueprint, fmt.Sprintf("feature.wrapper.%03d", index+1))
		if !exists || !strings.Contains(wrapper.Static, "<FeatureBoundary") {
			t.Fatalf("feature %d has no code-owned boundary: %#v", index+1, wrapper)
		}
	}
	acceptance, exists := directCodingTypeScriptBlueprintBlock(blueprint, "acceptance.001")
	if !exists || acceptance.FailureTarget != "feature.001" {
		t.Fatalf("independent acceptance does not target implementation: %#v", acceptance)
	}
	runtime, exists := directCodingTypeScriptBlueprintBlock(blueprint, "runtime.api")
	if !exists || !strings.Contains(runtime.Static, `"capability_001"`) ||
		!strings.Contains(runtime.Static, `"capability_002"`) ||
		!strings.Contains(runtime.Static, "Unknown application capability") {
		t.Fatalf("runtime omitted authoritative capability graph")
	}
	for _, required := range []string{"interface FeatureActions", "interface FeatureViewProps", "CapabilitySnapshot"} {
		if !strings.Contains(runtime.API, required) {
			t.Fatalf("model view API omitted %q:\n%s", required, runtime.API)
		}
	}
	for _, forbidden := range []string{"ApplicationRuntime", "FeatureRuntime", "useOwnCapabilityState", "publishCapability", "playTone", "useGlobalKeyboard"} {
		if strings.Contains(runtime.API, forbidden) {
			t.Fatalf("model view API exposed code-owned runtime %q:\n%s", forbidden, runtime.API)
		}
	}
	factory, exists := directCodingTypeScriptBlueprintBlock(blueprint, "runtime.factory")
	if !exists || !strings.Contains(factory.API, "createFeatureRuntime") {
		t.Fatalf("code-owned feature runtime factory is missing: %#v", factory)
	}
}
