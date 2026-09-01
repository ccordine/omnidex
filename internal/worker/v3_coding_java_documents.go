package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func genericJavaCommandLineDocuments(
	specification assemblyline.ApplicationSpecification,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	implementations := make([]assemblyline.SourceDocument, 0, len(specification.Requirements))
	applicationDependencies := []string{
		javaRuntimeFeatureResultBlock,
		javaRuntimeApplicationInspectBlock,
	}
	for index, requirement := range specification.Requirements {
		sequence := index + 1
		context, exists := contexts[requirement.ID]
		if !exists {
			return nil, fmt.Errorf("Java command-line workload omits requirement %s", requirement.ID)
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
		featureID := fmt.Sprintf("feature.%03d", sequence)
		featureClass := fmt.Sprintf("Feature%03d", sequence)
		featureMethod := fmt.Sprintf("feature%03d", sequence)
		supportID, supportSource, supportAPI := javaCommandLineCapabilityProjection(
			sequence, specification.Requirements, capabilities[requirement.ID],
		)
		dependencies := []string{javaRuntimeFeatureResultBlock}
		implementationBlocks := make([]assemblyline.SourceBlock, 0, 2)
		if supportID != "" {
			implementationBlocks = append(implementationBlocks, assemblyline.SourceBlock{
				ID: supportID, Static: supportSource, API: supportAPI,
				TaskID: context.Task.TaskID, Role: assemblyline.SourceBlockTaskSupport,
			})
			dependencies = append(dependencies, supportID)
			applicationDependencies = append(applicationDependencies, supportID)
		}
		featureSignature := fmt.Sprintf(
			"static Map<String, Object> %s(Map<String, Object> input, Map<String, Object> dependencies)",
			featureMethod,
		)
		implementationBlocks = append(implementationBlocks, assemblyline.SourceBlock{
			ID: featureID, Signature: featureSignature,
			Contract:     javaCommandLineFeatureContract(requirementBehavior),
			API:          javaCommandLineFeatureAPI(featureClass, featureSignature),
			DependsOn:    dependencies,
			Capabilities: append([]string(nil), dependencies...),
			Globals:      javaCommandLineFragmentGlobals(),
			TaskID:       context.Task.TaskID,
			Role:         assemblyline.SourceBlockTaskImplementation,
		})
		implementations = append(implementations, assemblyline.SourceDocument{
			ID: fmt.Sprintf("workload_implementation_%03d", sequence), Path: implementationPath,
			Preamble: javaCommandLineClassPreamble(featureClass), Postamble: "}",
			Blocks: implementationBlocks,
		})
		applicationDependencies = append(applicationDependencies, featureID)
	}
	order, err := goCommandLineRequirementOrder(specification.Requirements, capabilities)
	if err != nil {
		return nil, err
	}
	documents := []assemblyline.SourceDocument{javaCommandLineRuntimeDocument()}
	documents = append(documents, implementations...)
	documents = append(documents, javaCommandLineApplicationDocument(
		specification.Requirements, capabilities, order, applicationDependencies,
	))
	return documents, nil
}

func javaCommandLineFeatureAPI(className, signature string) string {
	return "final class " + className + " { " +
		strings.Replace(signature, "static ", "static native ", 1) + "; }"
}

func javaCommandLineClassPreamble(className string) string {
	return strings.Join([]string{
		"import java.util.List;", "import java.util.Map;", "",
		"@SuppressWarnings(\"auxiliaryclass\")", "final class " + className + " {",
	}, "\n")
}

func javaCommandLineFragmentGlobals() []string {
	return []string{"Map", "List", "String", "Integer", "Boolean", "Long", "Double"}
}

func javaCommandLineCapabilityProjection(
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
		name := fmt.Sprintf(
			"FEATURE_%03d_CAPABILITY_%03d", ownerSequence, indices[dependency.RequirementID],
		)
		lines = append(lines, fmt.Sprintf(
			"private static final String %s = %s;", name, strconv.Quote(dependency.CapabilityID),
		))
	}
	declaration := strings.Join(lines, "\n")
	return fmt.Sprintf("feature.capabilities.%03d", ownerSequence), declaration, declaration
}

func javaCommandLineFeatureContract(behavior string) string {
	return strings.TrimSpace(behavior)
}
