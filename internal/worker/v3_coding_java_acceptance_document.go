package worker

import (
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func javaCommandLineAcceptanceDocument(sequence int, taskID, behavior string, pair directCodingTaskArtifactPair) (assemblyline.SourceDocument, error) {
	className := strings.TrimSuffix(path.Base(pair.VerificationPath), ".java")
	if !javaSourceIdentifier(className) {
		return assemblyline.SourceDocument{}, fmt.Errorf("Java verification requires a valid code-owned class name")
	}
	observationID := fmt.Sprintf("acceptance.observe.%03d", sequence)
	acceptanceName := fmt.Sprintf("verifyFeature%03d", sequence)
	signature := "static void " + acceptanceName + "()"
	observationSignature := "static Map<String, Object> run(Map<String, Object> input, Map<String, Object> dependencies)"
	return assemblyline.SourceDocument{
		ID: fmt.Sprintf("workload_verification_%03d", sequence), Path: pair.VerificationPath,
		Preamble:  javaCommandLineClassPreamble(className),
		Postamble: "}",
		Blocks: []assemblyline.SourceBlock{
			{
				ID: observationID, Static: observationSignature + fmt.Sprintf(" { return Runtime.normalizeResult(Feature%03d.feature%03d(input, dependencies)); }", sequence, sequence),
				API: `/** Input: arguments: List<String>; standardInput: String.
 * Dependencies: direct capability IDs mapped to result maps.
 * Result: output: String; error: String; exitCode: Integer; state: Map<String, Object>.
 */
static native Map<String, Object> run(Map<String, Object> input, Map<String, Object> dependencies);`,
				DependsOn: []string{javaRuntimeNormalizeBlock, fmt.Sprintf("feature.%03d", sequence)},
				TaskID:    taskID, Role: assemblyline.SourceBlockTaskSupport,
			},
			{
				ID: fmt.Sprintf("acceptance.%03d", sequence), Signature: signature, API: signature,
				Contract: strings.Join([]string{
					strings.TrimSpace(behavior),
					"Observe this behavior with one representative literal input and dependency set. Demonstrate the requirement with direct assert comparisons between that observation and independently determined literal expected values.",
				}, "\n"),
				DependsOn: []string{observationID}, Capabilities: []string{observationID}, Globals: javaCommandLineFragmentGlobals(),
				TaskID: taskID, Role: assemblyline.SourceBlockTaskVerification,
			},
			{
				ID: fmt.Sprintf("acceptance.runner.%03d", sequence),
				Static: fmt.Sprintf(`public static void main(String[] arguments) {
  if (!%s.class.desiredAssertionStatus()) {
    throw new IllegalStateException("behavioral verification requires enabled assertions");
  }
  %s();
  System.out.println("%s passed");
}`, className, acceptanceName, acceptanceName),
				API: "public static void main(String[] arguments)", DependsOn: []string{fmt.Sprintf("acceptance.%03d", sequence)},
				TaskID: taskID, Role: assemblyline.SourceBlockTaskSupport,
			},
		},
	}, nil
}
