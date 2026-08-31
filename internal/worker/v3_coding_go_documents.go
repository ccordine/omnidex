package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericGoCommandLineDocuments(
	specification assemblyline.ApplicationSpecification,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	runtimeDocument, err := goCommandLineRuntimeDocument(nil)
	if err != nil {
		return nil, err
	}
	implementations := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	implementationByPath := make(map[string]int, len(specification.Requirements))
	applicationDependencies := []string{"runtime.api"}
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		context, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("Go command-line workload omits requirement %s", requirement.ID)
		}
		requirementBehavior, err := compileDirectCodingApplicationTaskBehavior(
			context, capabilities[requirement.ID],
		)
		if err != nil {
			return nil, err
		}
		implementationPath, err := directCodingTaskSingleImplementationPath(
			coverage, context.Task.TaskID,
		)
		if err != nil {
			return nil, err
		}
		implementationIndex, exists := implementationByPath[implementationPath]
		if !exists {
			implementationIndex = len(implementations)
			implementationByPath[implementationPath] = implementationIndex
			implementations = append(implementations, assemblyline.SourceDocument{
				ID:   fmt.Sprintf("workload_implementation_%03d", sequence),
				Path: implementationPath, Preamble: "package main",
			})
		}
		featureID := fmt.Sprintf("feature.%03d", sequence)
		featureName := fmt.Sprintf("Feature%03d", sequence)
		supportID, supportSource, supportAPI := goCommandLineCapabilityProjection(
			sequence, specification.Requirements, capabilities[requirement.ID],
		)
		dependencies := []string{"runtime.api"}
		if supportID != "" {
			implementations[implementationIndex].Blocks = append(
				implementations[implementationIndex].Blocks, assemblyline.SourceBlock{
					ID: supportID, Static: supportSource, API: supportAPI,
					DependsOn: []string{"runtime.api"}, TaskID: context.Task.TaskID,
					Role: assemblyline.SourceBlockTaskSupport,
				})
			dependencies = append(dependencies, supportID)
			applicationDependencies = append(applicationDependencies, supportID)
		}
		implementations[implementationIndex].Blocks = append(
			implementations[implementationIndex].Blocks, assemblyline.SourceBlock{
				ID: featureID,
				Signature: fmt.Sprintf(
					"func %s(input TaskInput, dependencies CapabilityResults) TaskResult", featureName,
				),
				Contract: goCommandLineFeatureContract(requirementBehavior),
				API: fmt.Sprintf(
					"func %s(input TaskInput, dependencies CapabilityResults) TaskResult", featureName,
				),
				DependsOn: dependencies, Capabilities: append([]string(nil), dependencies...),
				TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskImplementation,
			})
		applicationDependencies = append(applicationDependencies, featureID)
	}
	order, err := goCommandLineRequirementOrder(specification.Requirements, capabilities)
	if err != nil {
		return nil, err
	}
	documents := []assemblyline.SourceDocument{runtimeDocument}
	documents = append(documents, implementations...)
	documents = append(documents, goCommandLineApplicationDocument(
		specification.Requirements, capabilities, order, applicationDependencies,
	))
	return documents, nil
}

func goCommandLineCapabilityProjection(
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
		name := fmt.Sprintf("Feature%03dCapability%03d", ownerSequence, indices[dependency.RequirementID])
		lines = append(lines, fmt.Sprintf("%s = %s", name, strconv.Quote(dependency.CapabilityID)))
	}
	declaration := "const (\n\t" + strings.Join(lines, "\n\t") + "\n)"
	return fmt.Sprintf("feature.capabilities.%03d", ownerSequence), declaration, declaration
}

func goCommandLineFeatureContract(behavior string) string {
	return strings.Join([]string{
		behavior,
		"The function body fully implements the exact local behavior and returns one complete TaskResult derived from input and the declared direct dependency results.",
		"Use Output for user-visible text, Error for a clear failure, ExitCode for process status, and State for reusable string values.",
	}, "\n")
}
