package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestJavaRuntimeRejectsIncompleteNullAndWrongTypeResults(t *testing.T) {
	t.Parallel()
	document := javaCommandLineRuntimeDocument()
	composed, err := assemblyline.ComposeJavaDocument(
		document,
		assemblyline.SourceComposition{
			Generated: map[string]string{}, Interfaces: map[string]string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	classes := filepath.Join(root, "build", "classes")
	if err := os.MkdirAll(classes, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Runtime.java":                composed.Source,
		"RuntimeValidationProbe.java": javaRuntimeValidationProbeSource(),
		"TestRunner.java":             javaCommandLineTestRunnerSource(),
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	compileOutput, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingJavaStageTimeout,
		"javac", "--release", "21", "-Xlint:all", "-Werror", "-d", "build/classes",
		"Runtime.java", "RuntimeValidationProbe.java", "TestRunner.java",
	)
	if err != nil {
		t.Fatalf("compile Java runtime validation probe: %v\n%s", err, compileOutput)
	}
	runOutput, err := runDirectCodingStageCommand(
		context.Background(), root, directCodingJavaStageTimeout,
		"java", "-ea", "-cp", "build/classes", "TestRunner",
		"RuntimeValidationProbe", "verifyRuntimeValidation",
	)
	if err != nil {
		t.Fatalf("run Java runtime validation probe: %v\n%s", err, runOutput)
	}
}

func TestJavaRuntimeNormalizationSourceHasNoCoerciveFallbacks(t *testing.T) {
	t.Parallel()
	source := javaCommandLineRuntimeFeatureResultSource() +
		javaCommandLineRuntimeApplicationInspectSource()
	for _, fallback := range []string{
		"getOrDefault", "String.valueOf", "instanceof Number", "(Number)",
	} {
		if strings.Contains(source, fallback) {
			t.Fatalf("Java runtime retains coercive fallback %q", fallback)
		}
	}
}

func javaRuntimeValidationProbeSource() string {
	return `import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

final class RuntimeValidationProbe {
  @FunctionalInterface
  private interface Attempt { void run(); }

  private RuntimeValidationProbe() {}

  static void verifyRuntimeValidation() {
    Runtime.requireResult(Runtime.result("", "", 0, Map.of()));
    rejected(() -> Runtime.result(null, "", 0, Map.of()));
    rejected(() -> Runtime.result("", null, 0, Map.of()));
    rejected(() -> Runtime.result("", "", 0, null));
    rejected(() -> Runtime.normalizeResult(null));
    for (String key : List.of("output", "error", "exitCode", "state")) {
      rejected(() -> Runtime.normalizeResult(without(key)));
      rejected(() -> Runtime.normalizeResult(withNull(key)));
      rejected(() -> Runtime.normalizeResult(withWrongType(key)));
    }
    rejected(() -> Runtime.normalizeResult(withExtraKey()));
  }

  private static Map<String, Object> candidate() {
    Map<String, Object> candidate = new LinkedHashMap<>();
    candidate.put("output", "ready");
    candidate.put("error", "");
    candidate.put("exitCode", 0);
    candidate.put("state", Map.of("status", "ready"));
    return candidate;
  }

  private static Map<String, Object> without(String key) {
    Map<String, Object> candidate = candidate();
    candidate.remove(key);
    return candidate;
  }

  private static Map<String, Object> withNull(String key) {
    Map<String, Object> candidate = candidate();
    candidate.put(key, null);
    return candidate;
  }

  private static Map<String, Object> withWrongType(String key) {
    Map<String, Object> candidate = candidate();
    if (key.equals("output")) candidate.put(key, 7);
    if (key.equals("error")) candidate.put(key, false);
    if (key.equals("exitCode")) candidate.put(key, 0L);
    if (key.equals("state")) candidate.put(key, "not-a-map");
    return candidate;
  }

  private static Map<String, Object> withExtraKey() {
    Map<String, Object> candidate = candidate();
    candidate.put("extra", true);
    return candidate;
  }

  private static void rejected(Attempt attempt) {
    try {
      attempt.run();
    } catch (IllegalArgumentException expected) {
      return;
    }
    throw new AssertionError("invalid result was accepted");
  }
}
`
}
