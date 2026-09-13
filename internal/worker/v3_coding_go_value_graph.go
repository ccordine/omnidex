package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func goValueIndices(requirements []assemblyline.Requirement) map[string]int {
	indices := make(map[string]int, len(requirements))
	for index, requirement := range requirements {
		indices[requirement.ID] = index + 1
	}
	return indices
}

func goValueSignature(name, requirementID string, indices map[string]int, capabilities directCodingCapabilityGraph, kinds directCodingResultValueKindPlan, sources directCodingInputSourcePlan) (string, error) {
	kind, err := goCommandLineValueType(kinds[requirementID])
	if err != nil {
		return "", err
	}
	parameter, inputType, err := goInputParameter(sources[requirementID])
	if err != nil {
		return "", err
	}
	var parameters []string
	if parameter != "" {
		parameters = append(parameters, parameter+" "+inputType)
	}
	for _, dependency := range capabilities[requirementID] {
		sequence, exists := indices[dependency.RequirementID]
		if !exists {
			return "", fmt.Errorf("Go value dependency %s is absent", dependency.RequirementID)
		}
		dependencyType, err := goCommandLineValueType(kinds[dependency.RequirementID])
		if err != nil {
			return "", err
		}
		parameters = append(parameters, fmt.Sprintf("dependency%03d %s", sequence, dependencyType))
	}
	return "func " + name + "(" + strings.Join(parameters, ", ") + ") " + kind, nil
}

func goValueBehavior(context assemblyline.ApplicationTaskContext, indices map[string]int, dependencies []directCodingCapabilityBinding) (string, error) {
	behavior, err := compileDirectCodingApplicationTaskLocalBehavior(context, nil)
	if err != nil {
		return "", err
	}
	for _, dependency := range dependencies {
		if strings.TrimSpace(dependency.Purpose) == "" {
			return "", fmt.Errorf("Go dependency has no local purpose")
		}
		behavior += fmt.Sprintf("\nValue dependency%03d: %s", indices[dependency.RequirementID], dependency.Purpose)
	}
	return behavior, nil
}

func goValueCall(name, requirementID string, indices map[string]int, capabilities directCodingCapabilityGraph, sources directCodingInputSourcePlan) string {
	var arguments []string
	if sources[requirementID] != assemblyline.ApplicationInputNone {
		arguments = append(arguments, fmt.Sprintf("input%03d", indices[requirementID]))
	}
	for _, dependency := range capabilities[requirementID] {
		arguments = append(arguments, fmt.Sprintf("value%03d", indices[dependency.RequirementID]))
	}
	return name + "(" + strings.Join(arguments, ", ") + ")"
}

func goValueClosure(requirementID string, capabilities directCodingCapabilityGraph) map[string]bool {
	needed := make(map[string]bool)
	var visit func(string)
	visit = func(id string) {
		if needed[id] {
			return
		}
		needed[id] = true
		for _, dependency := range capabilities[id] {
			visit(dependency.RequirementID)
		}
	}
	visit(requirementID)
	return needed
}

func goValueInvocationLines(order []string, indices map[string]int, capabilities directCodingCapabilityGraph, needed map[string]bool, sources directCodingInputSourcePlan) ([]string, []string) {
	var lines, blockIDs []string
	for _, id := range order {
		if needed != nil && !needed[id] {
			continue
		}
		sequence := indices[id]
		lines = append(lines, fmt.Sprintf("value%03d := %s", sequence, goValueCall(fmt.Sprintf("Feature%03d", sequence), id, indices, capabilities, sources)))
		blockIDs = append(blockIDs, fmt.Sprintf("feature.%03d", sequence))
	}
	return lines, blockIDs
}
