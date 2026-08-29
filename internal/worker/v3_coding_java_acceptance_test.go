package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaAcceptanceRequiresExactFeatureAndResultDerivedRuntimeChecks(t *testing.T) {
	stage, ref := javaAcceptanceFixture(t)
	valid := `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require("ready".equals(result.get("output")), "output mismatch");
}`
	if err := validateDirectCodingJavaAcceptance(&stage, ref, valid); err != nil {
		t.Fatal(err)
	}
	negated := `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require(!"failure".equals(result.get("error")), "unexpected failure");
}`
	if err := validateDirectCodingJavaAcceptance(&stage, ref, negated); err != nil {
		t.Fatalf("rejected exact negated Java predicate: %v", err)
	}
	for name, source := range map[string]string{
		"wrong feature": `static void verifyFeature001() {
  Map<String, Object> result = Feature002.feature001(Map.of(), Map.of());
  Runtime.requireResult(result); Runtime.require("ready".equals(result.get("output")), "bad");
}`,
		"no shape check": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.require("ready".equals(result.get("output")), "bad");
}`,
		"shape check after condition": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.require("ready".equals(result.get("output")), "bad");
  Runtime.requireResult(result);
}`,
		"constant check": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result); Runtime.require(true, "bad " + result);
}`,
		"compound tautology": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require(result != null || true, "bad");
}`,
		"bitwise truth forcing": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require("impossible".equals(result.get("output")) | String.valueOf("x").equals("x"), "bad");
}`,
		"integer bitwise expected": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require(Integer.valueOf(1 | 2).equals(result.get("exitCode")), "bad");
}`,
		"cast wrapper": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require((boolean) "ready".equals(result.get("output")), "bad");
}`,
		"value wrapper": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require(Boolean.valueOf("ready".equals(result.get("output"))), "bad");
}`,
		"same value comparison": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require(result.get("output") == result.get("output"), "bad");
}`,
		"detached assertion": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Map<String, Object> detached = result;
  Runtime.requireResult(result);
  Runtime.require("ready".equals(detached.get("output")), "bad");
}`,
		"caught assertion": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  try { Runtime.require("ready".equals(result.get("output")), "bad"); }
  catch (AssertionError ignored) { }
}`,
		"dead branch assertion": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  if (false) { Runtime.require("ready".equals(result.get("output")), "bad"); }
}`,
		"loop assertion": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  for (int index = 0; index < 1; index++) {
    Runtime.require("ready".equals(result.get("output")), "bad");
  }
}`,
		"closure assertion": `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runnable assertion = () -> Runtime.require("ready".equals(result.get("output")), "bad");
}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := validateDirectCodingJavaAcceptance(&stage, ref, source)
			if err == nil {
				t.Fatalf("accepted invalid Java verification source:\n%s", source)
			}
			switch name {
			case "dead branch assertion", "loop assertion", "closure assertion":
				if !strings.Contains(err.Error(), "direct method-body statements") {
					t.Fatalf("nested assertion rejected for the wrong reason: %v", err)
				}
			}
		})
	}
}

func javaAcceptanceFixture(
	t *testing.T,
) (directCodingProgram, assemblyline.SourceBlockRef) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceCommandLine, ProductQuote: "verification fixture",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Return a ready result",
		}},
	}
	workload := freezeJavaWorkload(t, specification)
	feature := assemblyline.SourceBlock{
		ID:        "feature.001",
		Signature: "static Map<String, Object> feature001(Map<String, Object> input, Map<String, Object> dependencies)",
		Contract:  "Return a result.", API: "static Map<String, Object> feature001(Map<String, Object> input, Map<String, Object> dependencies)",
		TaskID: "task_001", Role: assemblyline.SourceBlockTaskImplementation,
	}
	acceptance := assemblyline.SourceBlock{
		ID: "acceptance.001", Signature: "static void verifyFeature001()", Contract: "Verify.",
		API: "static void verifyFeature001()", DependsOn: []string{"feature.001"},
		TaskID: "task_001", Role: assemblyline.SourceBlockTaskVerification,
	}
	documents := []assemblyline.SourceDocument{
		{ID: "feature", Path: "Echo.java", AdapterID: "java", Preamble: javaCommandLineClassPreamble("Feature001"), Postamble: "}", Blocks: []assemblyline.SourceBlock{feature}},
		{ID: "acceptance", Path: "EchoTest.java", AdapterID: "java", Preamble: javaCommandLineClassPreamble("FeatureTest001"), Postamble: "}", Blocks: []assemblyline.SourceBlock{acceptance}},
	}
	return directCodingProgram{
		StackID: genericJavaCommandLineAdapter, Workload: workload,
		Source: assemblyline.SourceBlueprint{Documents: documents},
	}, assemblyline.SourceBlockRef{Document: documents[1], Block: acceptance}
}
