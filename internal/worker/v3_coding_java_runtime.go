package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	javaRuntimeFeatureResultBlock      = "runtime.feature.result"
	javaRuntimeApplicationInspectBlock = "runtime.application.inspect"
)

func javaCommandLineRuntimeDocument() assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "application_runtime", Path: "Runtime.java",
		Preamble: strings.Join([]string{
			"import java.util.ArrayList;", "import java.util.LinkedHashMap;",
			"import java.util.List;", "import java.util.Map;", "", "final class Runtime {",
		}, "\n"),
		Postamble: "}",
		Blocks: []assemblyline.SourceBlock{
			{
				ID: javaRuntimeFeatureResultBlock, Static: javaCommandLineRuntimeFeatureResultSource(),
				API: javaCommandLineRuntimeFeatureResultAPI(),
			},
			{
				ID: javaRuntimeApplicationInspectBlock, Static: javaCommandLineRuntimeApplicationInspectSource(),
				API: javaCommandLineRuntimeApplicationInspectAPI(),
			},
		},
	}
}

func javaCommandLineRuntimeFeatureResultAPI() string {
	return `final class Runtime {
  static native Map<String, Object> result(String output, String error, int exitCode, Map<String, Object> state);
  static native Map<String, Object> dependency(Map<String, Object> dependencies, String capabilityID);
}`
}

func javaCommandLineRuntimeApplicationInspectAPI() string {
	return `final class Runtime {
  static native String output(Map<String, Object> result);
  static native String error(Map<String, Object> result);
  static native int exitCode(Map<String, Object> result);
  static native Map<String, Object> state(Map<String, Object> result);
}`
}

func javaCommandLineRuntimeFeatureResultSource() string {
	return `static Map<String, Object> input(String[] arguments, String standardInput) {
  Map<String, Object> input = new LinkedHashMap<>();
  input.put("arguments", new ArrayList<>(List.of(arguments)));
  input.put("standardInput", standardInput == null ? "" : standardInput);
  return input;
}

static Map<String, Object> result(
    String output, String error, int exitCode, Map<String, Object> state) {
  if (output == null) throw new IllegalArgumentException("feature result output must be a string");
  if (error == null) throw new IllegalArgumentException("feature result error must be a string");
  if (state == null) throw new IllegalArgumentException("feature result state must be a map");
  Map<String, Object> result = new LinkedHashMap<>();
  result.put("output", output);
  result.put("error", error);
  result.put("exitCode", exitCode);
  result.put("state", new LinkedHashMap<>(state));
  return result;
}

static Map<String, Object> normalizeResult(Map<String, Object> candidate) {
  if (candidate == null) throw new IllegalArgumentException("feature result must be a map");
  if (candidate.size() != 4 || !candidate.containsKey("output") ||
      !candidate.containsKey("error") || !candidate.containsKey("exitCode") ||
      !candidate.containsKey("state")) {
    throw new IllegalArgumentException("feature result requires exactly output, error, exitCode, and state");
  }
  Object output = candidate.get("output");
  Object error = candidate.get("error");
  Object exitCode = candidate.get("exitCode");
  Object state = candidate.get("state");
  if (!(output instanceof String)) {
    throw new IllegalArgumentException("feature result output must be a string");
  }
  if (!(error instanceof String)) {
    throw new IllegalArgumentException("feature result error must be a string");
  }
  if (!(exitCode instanceof Integer)) {
    throw new IllegalArgumentException("feature result exitCode must be an integer");
  }
  if (!(state instanceof Map<?, ?>)) {
    throw new IllegalArgumentException("feature result state must be a map");
  }
  return result(
      (String) output, (String) error, (Integer) exitCode, copyStringMap(state));
}

static Map<String, Object> dependency(
    Map<String, Object> dependencies, String capabilityID) {
  if (dependencies == null || !dependencies.containsKey(capabilityID)) {
    throw new IllegalArgumentException("missing direct capability " + capabilityID);
  }
  Object value = dependencies.get(capabilityID);
  if (!(value instanceof Map<?, ?>)) {
    throw new IllegalArgumentException("direct capability result must be a map");
  }
  return normalizeResult(copyStringMap(value));
}`
}

func javaCommandLineRuntimeApplicationInspectSource() string {
	return `static String output(Map<String, Object> result) {
  return (String) normalizeResult(result).get("output");
}

static String error(Map<String, Object> result) {
  return (String) normalizeResult(result).get("error");
}

static int exitCode(Map<String, Object> result) {
  return (Integer) normalizeResult(result).get("exitCode");
}

static Map<String, Object> state(Map<String, Object> result) {
  return copyStringMap(normalizeResult(result).get("state"));
}

static void mergeInto(Map<String, Object> combined, Map<String, Object> candidate) {
  Map<String, Object> part = normalizeResult(candidate);
  String partOutput = output(part);
  if (!partOutput.isEmpty()) {
    String current = output(combined);
    combined.put("output", current.isEmpty() ? partOutput : current + "\n" + partOutput);
  }
  Map<String, Object> combinedState = state(combined);
  combinedState.putAll(state(part));
  combined.put("state", combinedState);
  if (exitCode(part) != 0) combined.put("exitCode", exitCode(part));
  if (!error(part).isEmpty()) {
    combined.put("error", error(part));
    if (exitCode(combined) == 0) combined.put("exitCode", 1);
  }
}

private static Map<String, Object> copyStringMap(Object value) {
  if (!(value instanceof Map<?, ?> source)) {
    throw new IllegalArgumentException("map value is required");
  }
  Map<String, Object> copy = new LinkedHashMap<>();
  for (Map.Entry<?, ?> entry : source.entrySet()) {
    if (!(entry.getKey() instanceof String key)) {
      throw new IllegalArgumentException("map keys must be strings");
    }
    copy.put(key, entry.getValue());
  }
  return copy;
}`
}

func javaCommandLineApplicationDocument(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	order []string,
	dependencies []string,
) assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "application_entrypoint", Path: "Main.java",
		Preamble: strings.Join([]string{
			"import java.nio.charset.StandardCharsets;", "import java.util.LinkedHashMap;",
			"import java.util.Map;", "", "@SuppressWarnings(\"auxiliaryclass\")",
			"public final class Main {",
		}, "\n"),
		Postamble: "}",
		Blocks: []assemblyline.SourceBlock{{
			ID:        "application.run",
			Static:    javaCommandLineApplicationSource(requirements, capabilities, order),
			API:       "static Map<String, Object> runApplication(String[] arguments, String standardInput)",
			DependsOn: append([]string(nil), dependencies...),
		}},
	}
}

func javaCommandLineApplicationSource(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	order []string,
) string {
	indices := make(map[string]int, len(requirements))
	for index, requirement := range requirements {
		indices[requirement.ID] = index + 1
	}
	var source strings.Builder
	source.WriteString("static Map<String, Object> runApplication(String[] arguments, String standardInput) {\n")
	source.WriteString("  Map<String, Object> input = Runtime.input(arguments, standardInput);\n")
	source.WriteString("  Map<String, Object> results = new LinkedHashMap<>();\n")
	source.WriteString("  Map<String, Object> combined = Runtime.result(\"\", \"\", 0, new LinkedHashMap<>());\n")
	for _, requirementID := range order {
		sequence := indices[requirementID]
		source.WriteString(fmt.Sprintf("  Map<String, Object> direct%03d = new LinkedHashMap<>();\n", sequence))
		for _, dependency := range capabilities[requirementID] {
			source.WriteString(fmt.Sprintf(
				"  direct%03d.put(%s, results.get(%s));\n", sequence,
				strconv.Quote(dependency.CapabilityID), strconv.Quote(dependency.CapabilityID),
			))
		}
		source.WriteString(fmt.Sprintf(
			"  Map<String, Object> result%03d = Runtime.normalizeResult(Feature%03d.feature%03d(input, direct%03d));\n",
			sequence, sequence, sequence, sequence,
		))
		source.WriteString(fmt.Sprintf(
			"  results.put(%s, result%03d);\n", strconv.Quote(genericApplicationCapabilityID(sequence)), sequence,
		))
		source.WriteString(fmt.Sprintf("  Runtime.mergeInto(combined, result%03d);\n", sequence))
		source.WriteString("  if (!Runtime.error(combined).isEmpty()) return combined;\n")
	}
	source.WriteString("  return combined;\n}\n\n")
	source.WriteString(`public static void main(String[] arguments) throws Exception {
  String standardInput = new String(System.in.readAllBytes(), StandardCharsets.UTF_8);
  Map<String, Object> result = runApplication(arguments, standardInput);
  String output = Runtime.output(result);
  String error = Runtime.error(result);
  if (!output.isEmpty()) System.out.println(output);
  if (!error.isEmpty()) System.err.println(error);
  int exitCode = Runtime.exitCode(result);
  if (exitCode != 0) System.exit(exitCode);
}`)
	return source.String()
}
