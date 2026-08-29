package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericGoCommandLineDocuments(
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	runtimeDocument, err := goCommandLineRuntimeDocument(nil)
	if err != nil {
		return nil, err
	}
	implementations := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	verifications := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	implementationByPath := make(map[string]int, len(specification.Requirements))
	verificationByPath := make(map[string]int, len(specification.Requirements))
	applicationDependencies := []string{"runtime.api"}
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		context, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("Go command-line workload omits requirement %s", requirement.ID)
		}
		behavior, err := compileDirectCodingApplicationTaskBehavior(context, capabilities[requirement.ID])
		if err != nil {
			return nil, err
		}
		if skill, exists := skills[requirement.ID]; exists {
			behavior += "\nValidated procedure: " + skill.Procedure
		}
		pair, err := directCodingTaskSinglePair(coverage, context.Task.TaskID)
		if err != nil {
			return nil, err
		}
		implementationIndex, exists := implementationByPath[pair.ImplementationPath]
		if !exists {
			implementationIndex = len(implementations)
			implementationByPath[pair.ImplementationPath] = implementationIndex
			implementations = append(implementations, assemblyline.SourceDocument{
				ID:   fmt.Sprintf("workload_implementation_%03d", sequence),
				Path: pair.ImplementationPath, Preamble: "package main",
			})
		}
		verificationIndex, exists := verificationByPath[pair.VerificationPath]
		if !exists {
			verificationIndex = len(verifications)
			verificationByPath[pair.VerificationPath] = verificationIndex
			verifications = append(verifications, assemblyline.SourceDocument{
				ID:   fmt.Sprintf("workload_verification_%03d", sequence),
				Path: pair.VerificationPath, Preamble: "package main\n\nimport \"testing\"",
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
				Contract: goCommandLineFeatureContract(behavior),
				API: fmt.Sprintf(
					"func %s(input TaskInput, dependencies CapabilityResults) TaskResult", featureName,
				),
				DependsOn: dependencies, Capabilities: append([]string(nil), dependencies...),
				TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskImplementation,
			})
		acceptanceDependencies := []string{"runtime.api", featureID}
		verifications[verificationIndex].Blocks = append(
			verifications[verificationIndex].Blocks, assemblyline.SourceBlock{
				ID:           fmt.Sprintf("acceptance.%03d", sequence),
				Signature:    fmt.Sprintf("func Test%s(t *testing.T)", featureName),
				Contract:     goCommandLineAcceptanceContract(behavior, featureName),
				API:          fmt.Sprintf("func Test%s(t *testing.T)", featureName),
				DependsOn:    acceptanceDependencies,
				Capabilities: append([]string(nil), acceptanceDependencies...),
				Globals:      []string{"Fatal", "Fatalf", "Error", "Errorf"},
				TaskID:       context.Task.TaskID, Role: assemblyline.SourceBlockTaskVerification,
			})
		applicationDependencies = append(applicationDependencies, featureID)
	}
	order, err := goCommandLineRequirementOrder(specification.Requirements, capabilities)
	if err != nil {
		return nil, err
	}
	documents := []assemblyline.SourceDocument{runtimeDocument}
	documents = append(documents, implementations...)
	documents = append(documents, verifications...)
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

func goCommandLineAcceptanceContract(behavior, featureName string) string {
	return strings.Join([]string{
		behavior,
		"Call " + featureName + " with representative TaskInput and CapabilityResults values.",
		"The exact accepted requirement is proven by t.Fatal, t.Fatalf, t.Error, or t.Errorf applied to a value read from that exact call's TaskResult.",
	}, "\n")
}
