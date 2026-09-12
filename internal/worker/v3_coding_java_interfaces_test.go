package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaImplementationReceivesOnlyItsDirectDeclarations(t *testing.T) {
	program := javaDependentBehaviorFixture(t)
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskImplementation {
				continue
			}
			input, err := directCodingLanguageFragmentInput(&program, assemblyline.SourceBlockRef{Document: document, Block: block}, "java")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(input.Signature, "arguments: List<String>; standardInput: String") || strings.ContainsAny(input.Signature, "\r\n") {
				t.Fatalf("input declaration omits code-owned field semantics: %s", input.Signature)
			}
			capabilities := strings.Join(input.Capabilities, "\n")
			wantCount := 1
			if block.ID == "feature.002" {
				wantCount = 3
				if !strings.Contains(capabilities, "static native Map<String, Object> dependency(") || !strings.Contains(capabilities, "FEATURE_002_CAPABILITY_001") || !strings.Contains(capabilities, "output: String") {
					t.Fatalf("dependent feature lacks its direct result declaration: %s", capabilities)
				}
			} else if strings.Contains(capabilities, "dependency(") {
				t.Fatalf("independent feature received unused dependency authority: %s", capabilities)
			}
			if len(input.Capabilities) != wantCount || !strings.Contains(capabilities, "static native Map<String, Object> result(") {
				t.Fatalf("wrong direct declaration projection: %v", input.Capabilities)
			}
			for _, forbidden := range []string{"normalizeResult", "runApplication", "feature001(", "verifyFeature", "copyStringMap"} {
				if strings.Contains(capabilities, forbidden) {
					t.Errorf("unrelated %s leaked into feature declarations", forbidden)
				}
			}
		}
	}
}

func javaDependentBehaviorFixture(t *testing.T) directCodingProgram {
	t.Helper()
	return compiledLanguageBehaviorWorkloadFixture(t, genericJavaCommandLineAdapter, []compiledLanguageBehaviorFixture{
		{"Write ready to standard output.", `return Runtime.result("ready", "", 0, Map.of());`, javaAcceptanceFixtureBinding + `assert result.get("output").equals("ready");`},
		{"Return the first operation's output converted to uppercase.", `return Runtime.result(((String) Runtime.dependency(dependencies, FEATURE_002_CAPABILITY_001).get("output")).toUpperCase(), "", 0, Map.of());`, `Map<String, Object> result = run(Map.of("arguments", List.of(), "standardInput", ""), Map.of("capability_001", Map.of("output", "ready", "error", "", "exitCode", 0, "state", Map.of()))); assert result.get("output").equals("READY");`},
	}, directCodingCapabilityGraph{"requirement_001": nil, "requirement_002": {{RequirementID: "requirement_001", CapabilityID: "capability_001", Purpose: "Supplied text."}}})
}
