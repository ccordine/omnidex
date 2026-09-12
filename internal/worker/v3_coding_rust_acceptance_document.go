package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func rustCommandLineAcceptanceDocument(sequence int, taskID, behavior, implementationModule string, pair directCodingTaskArtifactPair) assemblyline.SourceDocument {
	featureID := fmt.Sprintf("feature.%03d", sequence)
	featureName := fmt.Sprintf("feature_%03d", sequence)
	observationID := fmt.Sprintf("acceptance.observe.%03d", sequence)
	acceptanceName := fmt.Sprintf("verify_feature_%03d", sequence)
	signature := "fn " + acceptanceName + "()"
	observationSignature := "fn run(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult"
	declarations := append(rustCommandLineRuntimeBlockIDs(), observationID)
	return assemblyline.SourceDocument{
		ID: fmt.Sprintf("workload_verification_%03d", sequence), Path: pair.VerificationPath,
		Preamble: strings.Join([]string{
			"use crate::runtime::{CapabilityResults, TaskInput, TaskResult};",
			fmt.Sprintf("use crate::%s::%s;", implementationModule, featureName),
			fmt.Sprintf("#[test]\nfn checks_feature_%03d() { %s(); }", sequence, acceptanceName),
		}, "\n"),
		Blocks: []assemblyline.SourceBlock{
			{
				ID: observationID, Static: observationSignature + " { " + featureName + "(input, dependencies) }",
				API: observationSignature, DependsOn: append(rustCommandLineRuntimeBlockIDs(), featureID),
				TaskID: taskID, Role: assemblyline.SourceBlockTaskSupport,
			},
			{
				ID: fmt.Sprintf("acceptance.%03d", sequence), Signature: signature, API: signature,
				Contract: strings.Join([]string{
					strings.TrimSpace(behavior),
					"Observe this behavior with one representative literal input and dependency set. Demonstrate the requirement with direct assert_eq! comparisons between that observation and independently determined literal expected values.",
				}, "\n"),
				DependsOn: declarations, Capabilities: append([]string(nil), declarations...), Globals: []string{"assert_eq"},
				TaskID: taskID, Role: assemblyline.SourceBlockTaskVerification,
			},
		},
	}
}
