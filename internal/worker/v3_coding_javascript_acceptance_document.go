package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func javaScriptCommandLineFeatureContract(behavior string) string {
	return strings.TrimSpace(behavior)
}

func javaScriptCommandLineAcceptanceDocument(sequence int, taskID, behavior string, pair directCodingTaskArtifactPair) assemblyline.SourceDocument {
	featureID := fmt.Sprintf("feature.%03d", sequence)
	featureName := fmt.Sprintf("feature%03d", sequence)
	acceptanceID := fmt.Sprintf("acceptance.%03d", sequence)
	observationID := fmt.Sprintf("acceptance.observe.%03d", sequence)
	acceptanceName := fmt.Sprintf("verifyFeature%03d", sequence)
	signature := "function " + acceptanceName + "()"
	return assemblyline.SourceDocument{
		ID: fmt.Sprintf("workload_verification_%03d", sequence), Path: pair.VerificationPath,
		Preamble: strings.Join([]string{
			"import assert from 'node:assert/strict';",
			"import test from 'node:test';",
			"import { normalizeTaskResult } from " + strconv.Quote(javaScriptRelativeModule(pair.VerificationPath, "runtime.mjs")) + ";",
			"import { " + featureName + " } from " + strconv.Quote(javaScriptRelativeModule(pair.VerificationPath, pair.ImplementationPath)) + ";",
			fmt.Sprintf("test(%s, %s);", strconv.Quote(featureID), acceptanceName),
		}, "\n"),
		Blocks: []assemblyline.SourceBlock{
			{
				ID:     observationID,
				Static: fmt.Sprintf("function run(input, dependencies) { return normalizeTaskResult(%s(input, dependencies)); }", featureName),
				API:    javaScriptCommandLineObservationAPI(), DependsOn: []string{"runtime.api", featureID},
				TaskID: taskID, Role: assemblyline.SourceBlockTaskSupport,
			},
			{
				ID: acceptanceID, Signature: signature, API: signature,
				Contract: strings.Join([]string{
					strings.TrimSpace(behavior),
					"Observe this behavior with one representative literal input and dependency set. Demonstrate the requirement with direct assert.strictEqual or assert.deepStrictEqual comparisons between that observation and independently determined literal expected values.",
				}, "\n"),
				DependsOn: []string{observationID}, Capabilities: []string{observationID}, Globals: []string{"assert"},
				TaskID: taskID, Role: assemblyline.SourceBlockTaskVerification,
			},
		},
	}
}
