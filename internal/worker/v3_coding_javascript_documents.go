package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericJavaScriptCommandLineDocuments(
	profile directCodingProjectVersionProfile,
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	implementations := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	verifications := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	implementationByPath := make(map[string]int, len(specification.Requirements))
	verificationByPath := make(map[string]int, len(specification.Requirements))
	applicationDependencies := []string{"runtime.api"}
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		context, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("JavaScript command-line workload omits requirement %s", requirement.ID)
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
		implementationIndex := sourceDocumentIndexForPath(
			&implementations, implementationByPath, pair.ImplementationPath,
			fmt.Sprintf("workload_implementation_%03d", sequence),
			"import { normalizeTaskResult } from './runtime.mjs';",
		)
		verificationIndex := sourceDocumentIndexForPath(
			&verifications, verificationByPath, pair.VerificationPath,
			fmt.Sprintf("workload_verification_%03d", sequence),
			"import test from 'node:test';\nimport assert from 'node:assert/strict';",
		)
		featureID := fmt.Sprintf("feature.%03d", sequence)
		featureName := fmt.Sprintf("feature%03d", sequence)
		supportID, supportSource, supportAPI := javaScriptCapabilityProjection(
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
				ID: featureID, Signature: "function " + featureName + "(input, dependencies)",
				Contract:  javaScriptCommandLineFeatureContract(behavior),
				API:       "function " + featureName + "(input, dependencies)",
				DependsOn: dependencies, Capabilities: append([]string(nil), dependencies...),
				Export: true,
				TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskImplementation,
			})
		verificationID := fmt.Sprintf("acceptance.%03d", sequence)
		verifyName := fmt.Sprintf("verifyFeature%03d", sequence)
		verifications[verificationIndex].ScopedPreambles = append(
			verifications[verificationIndex].ScopedPreambles, assemblyline.SourcePreamble{
				TaskID: context.Task.TaskID,
				Source: fmt.Sprintf("import { %s } from %s;", featureName,
					strconv.Quote(javaScriptRelativeModule(pair.VerificationPath, pair.ImplementationPath))),
			},
		)
		verifications[verificationIndex].Blocks = append(
			verifications[verificationIndex].Blocks,
			assemblyline.SourceBlock{
				ID: verificationID, Signature: "function " + verifyName + "()",
				Contract:     javaScriptCommandLineAcceptanceContract(behavior, featureName),
				API:          "function " + verifyName + "()",
				DependsOn:    append(append([]string(nil), dependencies...), featureID),
				Capabilities: []string{featureID}, Globals: []string{"assert"},
				TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskVerification,
			},
			assemblyline.SourceBlock{
				ID:        fmt.Sprintf("acceptance.register.%03d", sequence),
				Static:    fmt.Sprintf("test(%s, %s);", strconv.Quote(fmt.Sprintf("Feature %03d", sequence)), verifyName),
				API:       "registered independent acceptance for " + requirement.ID,
				DependsOn: []string{verificationID}, TaskID: context.Task.TaskID,
				Role: assemblyline.SourceBlockTaskSupport,
			},
		)
		applicationDependencies = append(applicationDependencies, featureID)
	}
	order, err := goCommandLineRequirementOrder(specification.Requirements, capabilities)
	if err != nil {
		return nil, err
	}
	runtime, err := javaScriptCommandLineRuntimeDocument(profile)
	if err != nil {
		return nil, err
	}
	documents := []assemblyline.SourceDocument{runtime}
	documents = append(documents, implementations...)
	documents = append(documents, verifications...)
	documents = append(documents, javaScriptCommandLineApplicationDocument(
		specification.Requirements, capabilities, order, coverage, contexts, applicationDependencies,
	))
	return documents, nil
}

func sourceDocumentIndexForPath(
	documents *[]assemblyline.SourceDocument,
	byPath map[string]int,
	artifactPath string,
	documentID string,
	preamble string,
) int {
	if index, exists := byPath[artifactPath]; exists {
		return index
	}
	index := len(*documents)
	byPath[artifactPath] = index
	*documents = append(*documents, assemblyline.SourceDocument{
		ID: documentID, Path: artifactPath, Preamble: preamble,
	})
	return index
}

func javaScriptCapabilityProjection(
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
		name := fmt.Sprintf("feature%03dCapability%03d", ownerSequence, indices[dependency.RequirementID])
		lines = append(lines, fmt.Sprintf("const %s = %s;", name, strconv.Quote(dependency.CapabilityID)))
	}
	declaration := strings.Join(lines, "\n")
	return fmt.Sprintf("feature.capabilities.%03d", ownerSequence), declaration, declaration
}

func javaScriptCommandLineFeatureContract(behavior string) string {
	return strings.Join([]string{
		behavior,
		"The function body fully implements the exact local behavior and returns one object with output, error, exitCode, and state fields derived from input and declared direct dependency results.",
	}, "\n")
}

func javaScriptCommandLineAcceptanceContract(behavior, featureName string) string {
	return strings.Join([]string{
		behavior,
		"Call " + featureName + " with representative input and dependency values.",
		"The exact accepted requirement is proven with node:assert/strict applied to a value read from that exact call's result.",
	}, "\n")
}
