package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericRustCommandLineDocuments(
	crateName string,
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	implementations := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	verifications := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	moduleBlocks := make([]assemblyline.SourceBlock, 0, len(specification.Requirements))
	modules := make(map[string]string, len(specification.Requirements))
	claimedPaths := make(map[string]string, len(specification.Requirements)*2)
	applicationDependencies := []string{"runtime.api"}
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		context, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("Rust command-line workload omits requirement %s", requirement.ID)
		}
		behavior, err := compileDirectCodingApplicationTaskBehavior(context, capabilities[requirement.ID])
		if err != nil {
			return nil, err
		}
		if skill, exists := skills[requirement.ID]; exists {
			behavior += "\nValidated procedure: " + skill.Procedure
		}
		pair, err := rustCommandLineTaskPair(coverage, context.Task.TaskID)
		if err != nil {
			return nil, err
		}
		for _, artifactPath := range []string{pair.ImplementationPath, pair.VerificationPath} {
			if owner, duplicate := claimedPaths[artifactPath]; duplicate {
				return nil, fmt.Errorf(
					"Rust path %s is shared by tasks %s and %s; this stack requires isolated modules",
					artifactPath, owner, context.Task.TaskID,
				)
			}
			claimedPaths[artifactPath] = context.Task.TaskID
		}
		module, err := rustCommandLineModuleForPath(pair.ImplementationPath)
		if err != nil {
			return nil, err
		}
		modules[requirement.ID] = module
		moduleBlocks = append(moduleBlocks, assemblyline.SourceBlock{
			ID:     fmt.Sprintf("library.module.%03d", sequence),
			Static: "pub mod " + module + ";", API: "pub mod " + module + ";",
			TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskSupport,
		})
		featureID := fmt.Sprintf("feature.%03d", sequence)
		featureName := fmt.Sprintf("feature_%03d", sequence)
		supportID, supportSource, supportAPI := rustCommandLineCapabilityProjection(
			sequence, specification.Requirements, capabilities[requirement.ID],
		)
		dependencies := []string{"runtime.api"}
		blocks := make([]assemblyline.SourceBlock, 0, 2)
		if supportID != "" {
			blocks = append(blocks, assemblyline.SourceBlock{
				ID: supportID, Static: supportSource, API: supportAPI,
				DependsOn: []string{"runtime.api"}, TaskID: context.Task.TaskID,
				Role: assemblyline.SourceBlockTaskSupport,
			})
			dependencies = append(dependencies, supportID)
			applicationDependencies = append(applicationDependencies, supportID)
		}
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: featureID,
			Signature: fmt.Sprintf(
				"pub fn %s(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult", featureName,
			),
			Contract:     rustCommandLineFeatureContract(behavior),
			API:          fmt.Sprintf("pub fn %s(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult", featureName),
			DependsOn:    dependencies,
			Capabilities: append([]string(nil), dependencies...),
			Globals:      []string{"String", "Vec"},
			TaskID:       context.Task.TaskID,
			Role:         assemblyline.SourceBlockTaskImplementation,
		})
		implementations = append(implementations, assemblyline.SourceDocument{
			ID:       fmt.Sprintf("workload_implementation_%03d", sequence),
			Path:     pair.ImplementationPath,
			Preamble: "use crate::runtime::{CapabilityResults, TaskInput, TaskResult};",
			Blocks:   blocks,
		})
		verificationID := fmt.Sprintf("acceptance.%03d", sequence)
		verifyName := fmt.Sprintf("verify_feature_%03d", sequence)
		verifications = append(verifications, rustCommandLineVerificationDocument(
			crateName, sequence, context.Task.TaskID, pair.VerificationPath,
			module, featureID, featureName, verificationID, verifyName, behavior,
			capabilities[requirement.ID],
		))
		applicationDependencies = append(applicationDependencies, featureID)
	}
	order, err := goCommandLineRequirementOrder(specification.Requirements, capabilities)
	if err != nil {
		return nil, err
	}
	documents := []assemblyline.SourceDocument{rustCommandLineRuntimeDocument()}
	documents = append(documents, implementations...)
	documents = append(documents, verifications...)
	documents = append(documents, rustCommandLineLibraryDocument(
		specification.Requirements, capabilities, order, modules,
		moduleBlocks, applicationDependencies,
	))
	documents = append(documents, rustCommandLineMainDocument(crateName))
	return documents, nil
}

func rustCommandLineVerificationDocument(
	crateName string,
	sequence int,
	taskID string,
	artifactPath string,
	module string,
	featureID string,
	featureName string,
	verificationID string,
	verifyName string,
	behavior string,
	capabilities []directCodingCapabilityBinding,
) assemblyline.SourceDocument {
	fixtureID := fmt.Sprintf("acceptance.fixture.%03d", sequence)
	fixtureName := fmt.Sprintf("representative_capability_results_for_feature_%03d", sequence)
	preamble := fmt.Sprintf(
		"use %s::{%s::%s, CapabilityResults, TaskInput, TaskResult};",
		crateName, module, featureName,
	)
	return assemblyline.SourceDocument{
		ID:   fmt.Sprintf("workload_verification_%03d", sequence),
		Path: artifactPath, Preamble: preamble,
		Blocks: []assemblyline.SourceBlock{
			{
				ID: fixtureID, Static: rustCommandLineCapabilityFixture(fixtureName, capabilities),
				API:       "fn " + fixtureName + "() -> CapabilityResults",
				DependsOn: []string{"runtime.api"}, TaskID: taskID,
				Role: assemblyline.SourceBlockTaskSupport,
			},
			{
				ID: verificationID, Signature: "fn " + verifyName + "()",
				Contract:     rustCommandLineAcceptanceContract(behavior, featureName, fixtureName),
				API:          "fn " + verifyName + "()",
				DependsOn:    []string{"runtime.api", featureID, fixtureID},
				Capabilities: []string{"runtime.api", featureID, fixtureID},
				Globals:      []string{"String", "Vec", "assert", "assert_eq", "assert_ne"},
				TaskID:       taskID, Role: assemblyline.SourceBlockTaskVerification,
			},
			{
				ID:        fmt.Sprintf("acceptance.register.%03d", sequence),
				Static:    fmt.Sprintf("#[test]\nfn test_feature_%03d() {\n    %s();\n}", sequence, verifyName),
				API:       "registered independent Rust acceptance for task " + taskID,
				DependsOn: []string{verificationID}, TaskID: taskID,
				Role: assemblyline.SourceBlockTaskSupport,
			},
		},
	}
}

func rustCommandLineCapabilityFixture(
	name string,
	capabilities []directCodingCapabilityBinding,
) string {
	var source strings.Builder
	source.WriteString("fn " + name + "() -> CapabilityResults {\n")
	source.WriteString("    let mut results = CapabilityResults::new();\n")
	for _, capability := range capabilities {
		quoted := strconv.Quote(capability.CapabilityID)
		source.WriteString("    results.insert(" + quoted + ".to_string(), TaskResult {\n")
		source.WriteString("        output: " + quoted + ".to_string(),\n")
		source.WriteString("        state: [(\"value\".to_string(), " + quoted + ".to_string())].into_iter().collect(),\n")
		source.WriteString("        ..TaskResult::default()\n")
		source.WriteString("    });\n")
	}
	source.WriteString("    results\n}")
	return source.String()
}

func rustCommandLineCapabilityProjection(
	ownerSequence int,
	requirements []assemblyline.Requirement,
	dependencies []directCodingCapabilityBinding,
) (string, string, string) {
	if len(dependencies) == 0 {
		return "", "", ""
	}
	indices := make(map[string]int, len(requirements))
	for index, requirement := range requirements {
		indices[requirement.ID] = index + 1
	}
	lines := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		name := fmt.Sprintf("FEATURE_%03d_CAPABILITY_%03d", ownerSequence, indices[dependency.RequirementID])
		lines = append(lines, fmt.Sprintf(
			"pub const %s: &str = %s;", name, strconv.Quote(dependency.CapabilityID),
		))
	}
	declaration := strings.Join(lines, "\n")
	return fmt.Sprintf("feature.capabilities.%03d", ownerSequence), declaration, declaration
}

func rustCommandLineFeatureContract(behavior string) string {
	return strings.Join([]string{
		behavior,
		"The function body fully implements the exact local behavior and returns one complete TaskResult derived from input and the declared direct dependency results.",
		"Use output for user-visible text, error for a clear failure, exit_code for process status, and state for reusable string values.",
	}, "\n")
}

func rustCommandLineAcceptanceContract(behavior, featureName, fixtureName string) string {
	return strings.Join([]string{
		behavior,
		"Call " + featureName + " with a representative TaskInput and " + fixtureName + "().",
		"The fixture contains one successful TaskResult for every declared direct capability; each result's output and state value equal that capability ID.",
		"Store its TaskResult in one local binding. Prove the exact accepted requirement with one assert_eq! or assert_ne! using a direct result.output, result.error, result.exit_code, result.state, result-field get, or result-field len observation first and one inline expected value second; assert! may contain one direct result-field comparison or one direct is_empty, contains, starts_with, ends_with, or contains_key test.",
	}, "\n")
}
