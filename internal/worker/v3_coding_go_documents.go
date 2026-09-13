package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericGoCommandLineDocuments(specification assemblyline.ApplicationSpecification, contexts map[string]assemblyline.ApplicationTaskContext, capabilities directCodingCapabilityGraph, coverage assemblyline.ApplicationFileCoveragePlan, valueKinds directCodingResultValueKindPlan, inputSources directCodingInputSourcePlan) ([]assemblyline.SourceDocument, error) {
	indices := goValueIndices(specification.Requirements)
	order, err := goCommandLineRequirementOrder(specification.Requirements, capabilities)
	if err != nil {
		return nil, err
	}
	implementations := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	verifications := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	implementationByPath, verificationByPath := map[string]int{}, map[string]int{}
	applicationDependencies := []string{"runtime.api"}
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		context, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("Go workload omits requirement %s", requirement.ID)
		}
		behavior, err := goValueBehavior(context, indices, capabilities[requirement.ID])
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("Feature%03d", sequence)
		signature, err := goValueSignature(name, requirement.ID, indices, capabilities, valueKinds, inputSources)
		if err != nil {
			return nil, err
		}
		pair, err := directCodingTaskSinglePair(coverage, context.Task.TaskID)
		if err != nil {
			return nil, err
		}
		implementationIndex, exists := implementationByPath[pair.ImplementationPath]
		if !exists {
			implementationIndex = len(implementations)
			implementationByPath[pair.ImplementationPath] = implementationIndex
			implementations = append(implementations, assemblyline.SourceDocument{ID: fmt.Sprintf("workload_implementation_%03d", sequence), Path: pair.ImplementationPath, Preamble: "package main"})
		}
		verificationIndex, exists := verificationByPath[pair.VerificationPath]
		if !exists {
			verificationIndex = len(verifications)
			verificationByPath[pair.VerificationPath] = verificationIndex
			verifications = append(verifications, assemblyline.SourceDocument{ID: fmt.Sprintf("workload_verification_%03d", sequence), Path: pair.VerificationPath, Preamble: "package main\n\nimport \"testing\""})
		}
		featureID := fmt.Sprintf("feature.%03d", sequence)
		dependencies := []string{"runtime.api"}
		for _, dependency := range capabilities[requirement.ID] {
			dependencies = append(dependencies, fmt.Sprintf("feature.%03d", indices[dependency.RequirementID]))
		}
		implementations[implementationIndex].Blocks = append(implementations[implementationIndex].Blocks, assemblyline.SourceBlock{
			ID: featureID, Signature: signature, API: signature,
			Contract:  behavior + "\n\nCompute the result value for input.",
			DependsOn: dependencies, Capabilities: goInputCapabilities(inputSources[requirement.ID]),
			TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskImplementation,
		})
		verificationBlocks, err := goCommandLineVerificationBlocks(sequence, context.Task.TaskID, requirement.ID, behavior, name, indices, capabilities, valueKinds, order, inputSources)
		if err != nil {
			return nil, err
		}
		verifications[verificationIndex].Blocks = append(verifications[verificationIndex].Blocks, verificationBlocks...)
		applicationDependencies = append(applicationDependencies, featureID)
	}
	documents := []assemblyline.SourceDocument{goCommandLineRuntimeDocument()}
	documents = append(documents, implementations...)
	documents = append(documents, verifications...)
	entrypoint, err := goCommandLineApplicationDocument(specification.Requirements, capabilities, valueKinds, order, applicationDependencies, inputSources)
	if err != nil {
		return nil, err
	}
	return append(documents, entrypoint), nil
}
