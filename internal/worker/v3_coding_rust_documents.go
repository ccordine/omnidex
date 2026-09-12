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
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	implementations := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	verifications := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	moduleBlocks := make([]assemblyline.SourceBlock, 0, len(specification.Requirements))
	modules := make(map[string]string, len(specification.Requirements))
	claimedPaths := make(map[string]string, len(specification.Requirements))
	applicationDependencies := rustCommandLineRuntimeBlockIDs()
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		context, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("Rust command-line workload omits requirement %s", requirement.ID)
		}
		requirementBehavior, err := compileDirectCodingApplicationTaskBehavior(
			context, capabilities[requirement.ID],
		)
		if err != nil {
			return nil, err
		}
		pair, err := directCodingTaskSinglePair(coverage, context.Task.TaskID)
		if err != nil {
			return nil, err
		}
		implementationPath := pair.ImplementationPath
		if owner, duplicate := claimedPaths[implementationPath]; duplicate {
			return nil, fmt.Errorf(
				"Rust path %s is shared by tasks %s and %s; this stack requires isolated modules",
				implementationPath, owner, context.Task.TaskID,
			)
		}
		claimedPaths[implementationPath] = context.Task.TaskID
		module, err := rustCommandLineModuleForPath(implementationPath)
		if err != nil {
			return nil, err
		}
		verificationModule, err := rustCommandLineModuleForPath(pair.VerificationPath)
		if err != nil {
			return nil, err
		}
		modules[requirement.ID] = module
		moduleBlocks = append(moduleBlocks, assemblyline.SourceBlock{
			ID:     fmt.Sprintf("library.module.%03d", sequence),
			Static: "pub mod " + module + ";", API: "pub mod " + module + ";",
			TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskSupport,
		})
		moduleBlocks = append(moduleBlocks, assemblyline.SourceBlock{
			ID:     fmt.Sprintf("library.verification_module.%03d", sequence),
			Static: "#[cfg(test)]\nmod " + verificationModule + ";", API: "mod " + verificationModule + ";",
			TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskSupport,
		})
		featureID := fmt.Sprintf("feature.%03d", sequence)
		featureName := fmt.Sprintf("feature_%03d", sequence)
		supportID, supportSource, supportAPI := rustCommandLineCapabilityProjection(
			sequence, specification.Requirements, capabilities[requirement.ID],
		)
		dependencies := rustCommandLineRuntimeBlockIDs()
		blocks := make([]assemblyline.SourceBlock, 0, 2)
		if supportID != "" {
			blocks = append(blocks, assemblyline.SourceBlock{
				ID: supportID, Static: supportSource, API: supportAPI,
				TaskID: context.Task.TaskID,
				Role:   assemblyline.SourceBlockTaskSupport,
			})
			dependencies = append(dependencies, supportID)
			applicationDependencies = append(applicationDependencies, supportID)
		}
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: featureID,
			Signature: fmt.Sprintf(
				"pub fn %s(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult", featureName,
			),
			Contract:     rustCommandLineFeatureContract(requirementBehavior),
			API:          fmt.Sprintf("pub fn %s(input: &TaskInput, dependencies: &CapabilityResults) -> TaskResult", featureName),
			DependsOn:    dependencies,
			Capabilities: append([]string(nil), dependencies...),
			Globals:      []string{"String", "Vec"},
			TaskID:       context.Task.TaskID,
			Role:         assemblyline.SourceBlockTaskImplementation,
		})
		implementations = append(implementations, assemblyline.SourceDocument{
			ID:       fmt.Sprintf("workload_implementation_%03d", sequence),
			Path:     implementationPath,
			Preamble: "use crate::runtime::{CapabilityResults, TaskInput, TaskResult};",
			Blocks:   blocks,
		})
		applicationDependencies = append(applicationDependencies, featureID)
		verifications = append(verifications, rustCommandLineAcceptanceDocument(sequence, context.Task.TaskID, requirementBehavior, module, pair))
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
	return strings.TrimSpace(behavior)
}
