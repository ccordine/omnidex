package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// Models supply example data and a required value independently. Code owns
// invocation, value comparison, test declarations, and failure reporting.
func goCommandLineVerificationBlocks(sequence int, taskID, requirementID, behavior, featureName string, indices map[string]int, capabilities directCodingCapabilityGraph, kinds directCodingResultValueKindPlan, order []string, sources directCodingInputSourcePlan) ([]assemblyline.SourceBlock, error) {
	inputID := fmt.Sprintf("acceptance.input.%03d", sequence)
	expectedID := fmt.Sprintf("acceptance.%03d", sequence)
	inputName := "MakeExampleInput" + featureName
	expectedName := "Expected" + featureName
	_, inputType, err := goInputParameter(sources[requirementID])
	if err != nil {
		return nil, err
	}
	inputSignature := "func " + inputName + "() " + inputType
	expectedSignature, err := goValueSignature(expectedName, requirementID, indices, capabilities, kinds, sources)
	if err != nil {
		return nil, err
	}
	needed := goValueClosure(requirementID, capabilities)
	invocations, dependencyIDs := goValueInvocationLines(order, indices, capabilities, needed, sources)
	var driver []string
	dependencies := []string{"runtime.api", expectedID}
	for _, id := range order {
		if !needed[id] || sources[id] == assemblyline.ApplicationInputNone {
			continue
		}
		number := indices[id]
		driver = append(driver, fmt.Sprintf("input%03d := MakeExampleInputFeature%03d()", number, number))
		dependencies = append(dependencies, fmt.Sprintf("acceptance.input.%03d", number))
	}
	driver = append(driver, invocations...)
	driver = append(driver, "expected := "+goValueCall(expectedName, requirementID, indices, capabilities, sources))
	driver = append(driver, fmt.Sprintf("if value%03d != expected { t.Fatalf(\"behavior mismatch: observed %%v; expected %%v\", value%03d, expected) }", sequence, sequence))
	dependencies = append(dependencies, dependencyIDs...)
	expectedDependencies := []string{"runtime.api"}
	for _, dependency := range capabilities[requirementID] {
		expectedDependencies = append(expectedDependencies, fmt.Sprintf("feature.%03d", indices[dependency.RequirementID]))
	}
	example := assemblyline.SourceBlock{ID: inputID, Signature: inputSignature, API: inputSignature,
		Contract:  behavior + "\n\nConstruct one example input value that could be supplied for this behavior.",
		DependsOn: []string{"runtime.api"}, Capabilities: goInputCapabilities(sources[requirementID]),
		TaskID: taskID, Role: assemblyline.SourceBlockTaskExample}

	blocks := []assemblyline.SourceBlock{
		example,
		{ID: expectedID, Signature: expectedSignature, API: expectedSignature,
			Contract:  behavior + "\n\nCompute the required result value for the supplied input.",
			DependsOn: expectedDependencies, Capabilities: goInputCapabilities(sources[requirementID]),
			TaskID: taskID, Role: assemblyline.SourceBlockTaskVerification},
		{ID: fmt.Sprintf("acceptance.driver.%03d", sequence),
			Static: "func Test" + featureName + "(t *testing.T) {\n" + strings.Join(driver, "\n") + "\n}",
			API:    "func Test" + featureName + "(t *testing.T)", DependsOn: dependencies,
			TaskID: taskID, Role: assemblyline.SourceBlockTaskSupport},
	}
	if sources[requirementID] == assemblyline.ApplicationInputNone {
		return blocks[1:], nil
	}
	return blocks, nil
}
