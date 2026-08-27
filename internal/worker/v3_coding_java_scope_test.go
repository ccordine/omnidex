package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaFragmentScopeAcceptsOnlyDeclaredTaskLocalAuthority(t *testing.T) {
	input := javaFeatureScopeInput()
	candidate := `static Map<String, Object> feature001(Map<String, Object> input, Map<String, Object> dependencies) {
  Object values = input.get("arguments");
  if (!(values instanceof List<?> arguments) || arguments.isEmpty()) {
    return Runtime.result("", "one argument is required", 2, Map.of());
  }
  String value = String.valueOf(arguments.get(0));
  return Runtime.result(value, "", 0, Map.<String, Object>of("value", value));
}`
	if _, err := validateDirectCodingJavaFragment(input, candidate); err != nil {
		t.Fatal(err)
	}

	verification := javaAcceptanceScopeInput()
	verificationSource := `static void verifyFeature001() {
  Map<String, Object> result = Feature001.feature001(Map.of(), Map.of());
  Runtime.requireResult(result);
  Runtime.require(String.valueOf(result.get("output")).isEmpty(), "unexpected output");
}`
	if _, err := validateDirectCodingJavaFragment(verification, verificationSource); err != nil {
		t.Fatal(err)
	}
}

func TestJavaFragmentScopeKeepsRuntimeRolesDisjoint(t *testing.T) {
	feature := javaFeatureScopeInput()
	for _, call := range []string{
		`Runtime.require(true, "unavailable");`,
		`Runtime.requireResult(Map.of());`,
		`Runtime.output(Map.of());`,
	} {
		candidate := feature.Signature + " {\n" + call +
			"\nreturn Runtime.result(\"\", \"\", 0, Map.of());\n}"
		if _, err := validateDirectCodingJavaFragment(feature, candidate); err == nil {
			t.Fatalf("Java feature acquired another runtime role:\n%s", candidate)
		}
	}

	acceptance := javaAcceptanceScopeInput()
	for _, call := range []string{
		`Runtime.result("", "", 0, Map.of());`,
		`Runtime.dependency(Map.of(), "hidden");`,
		`Runtime.output(Map.of());`,
	} {
		candidate := acceptance.Signature + " {\n" + call + "\n}"
		if _, err := validateDirectCodingJavaFragment(acceptance, candidate); err == nil {
			t.Fatalf("Java acceptance acquired another runtime role:\n%s", candidate)
		}
	}
}

func TestJavaFragmentScopeRejectsEnvironmentAndUndeclaredAuthority(t *testing.T) {
	input := javaFeatureScopeInput()
	for name, statement := range map[string]string{
		"system":                          `System.getenv("SECRET");`,
		"runtime process":                 `Runtime.getRuntime();`,
		"process builder":                 `new ProcessBuilder("command");`,
		"filesystem files":                `Files.readString(Path.of("value"));`,
		"filesystem file":                 `new File("value");`,
		"socket":                          `new Socket("localhost", 1);`,
		"url":                             `new URL("https://example.invalid");`,
		"reflection":                      `Class.forName("Hidden");`,
		"class literal protection domain": `Runtime.class.getProtectionDomain().getCodeSource().getLocation();`,
		"class literal loader":            `Runtime.class.getClassLoader();`,
		"bound get class":                 `input.getClass();`,
		"bound arbitrary method":          `input.hashCode();`,
		"bound resource method":           `input.getResource("hidden");`,
		"object result method":            `Object value = input.get("value"); value.toString();`,
		"unqualified API method":          `result("", "", 0, Map.of());`,
		"wrong API arity":                 `Runtime.result("", "", 0);`,
		"wrong pure method arity":         `input.isEmpty("extra");`,
		"unsafe":                          `Unsafe.getUnsafe();`,
		"undeclared class":                `Mystery.run();`,
		"undeclared method":               `escape();`,
		"local class":                     `class Hidden { }`,
	} {
		t.Run(name, func(t *testing.T) {
			candidate := strings.Join([]string{
				input.Signature + " {", statement,
				`return Runtime.result("", "", 0, Map.of());`, "}",
			}, "\n")
			if _, err := validateDirectCodingJavaFragment(input, candidate); err == nil {
				t.Fatalf("accepted forbidden Java authority:\n%s", candidate)
			}
		})
	}
}

func TestJavaFragmentScopeRejectsExtraTopLevelDeclarations(t *testing.T) {
	input := javaFeatureScopeInput()
	candidate := `import java.io.File;
static Map<String, Object> feature001(Map<String, Object> input, Map<String, Object> dependencies) {
  return Runtime.result("", "", 0, Map.of());
}`
	if _, err := validateDirectCodingJavaFragment(input, candidate); err == nil {
		t.Fatal("accepted an import beside the bounded Java method")
	}
}

func TestJavaRegisteredTargetTreeRestrictsOneRootUnreservedPair(t *testing.T) {
	stack, err := directCodingProjectStackByID(genericJavaCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	valid := assemblyline.TargetTree{
		StackID: genericJavaCommandLineAdapter, Paths: []string{"Behavior.java", "BehaviorTest.java"},
	}
	if err := validateDirectCodingFocusedTargetTree(stack, valid); err != nil {
		t.Fatal(err)
	}
	for name, paths := range map[string][]string{
		"nested":   {"src/Behavior.java", "BehaviorTest.java"},
		"reserved": {"Main.java", "BehaviorTest.java"},
		"unpaired": {"First.java", "Second.java"},
		"extra":    {"Behavior.java", "BehaviorTest.java", "OtherTest.java"},
	} {
		t.Run(name, func(t *testing.T) {
			target := assemblyline.TargetTree{StackID: genericJavaCommandLineAdapter, Paths: paths}
			if err := validateDirectCodingFocusedTargetTree(stack, target); err == nil {
				t.Fatalf("accepted invalid Java focused tree %v", paths)
			}
		})
	}
}

func javaFeatureScopeInput() assemblyline.FragmentGenerationInput {
	signature := "static Map<String, Object> feature001(Map<String, Object> input, Map<String, Object> dependencies)"
	return assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21", Signature: signature, Behavior: "Return a normalized result.",
		PermittedSymbols: []string{
			javaCommandLineRuntimeFeatureResultAPI(),
			"Map", "List", "String", "Object",
			"Integer", "Boolean", "Long", "Double",
		},
	}
}

func javaAcceptanceScopeInput() assemblyline.FragmentGenerationInput {
	feature := javaFeatureScopeInput()
	return assemblyline.FragmentGenerationInput{
		Language: "java", Dialect: "Java 21", Signature: "static void verifyFeature001()", Behavior: "Verify.",
		PermittedSymbols: []string{
			javaCommandLineRuntimeAcceptanceAssertAPI(),
			javaCommandLineFeatureAPI("Feature001", feature.Signature),
			"Map", "List", "String",
		},
	}
}
