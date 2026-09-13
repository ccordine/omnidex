package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const goCommandLineInputType = `type TaskInput struct {
	Arguments     []string // Values the user supplies as command-line arguments, excluding the executable name.
	StandardInput string   // Text for behavior that explicitly consumes redirected or piped input.
}`

func goCommandLineRuntimeDocument() assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "application_runtime", Path: "runtime.go", Preamble: "package main",
		Blocks: []assemblyline.SourceBlock{{ID: "runtime.api", Static: goCommandLineInputType, API: goCommandLineInputType}},
	}
}

func goCommandLineApplicationDocument(requirements []assemblyline.Requirement, capabilities directCodingCapabilityGraph, kinds directCodingResultValueKindPlan, order, dependencies []string, sources directCodingInputSourcePlan) (assemblyline.SourceDocument, error) {
	source, err := goCommandLineApplicationSource(requirements, capabilities, kinds, order, sources)
	if err != nil {
		return assemblyline.SourceDocument{}, err
	}
	imports := []string{"\"fmt\"", "\"os\""}
	if goNeedsStandardInput(sources) {
		imports = append(imports, "\"io\"")
	}
	for _, requirement := range requirements {
		if kinds[requirement.ID] != assemblyline.ApplicationResultText {
			imports = append(imports, "\"strconv\"")
			break
		}
	}
	return assemblyline.SourceDocument{
		ID: "application_entrypoint", Path: "main.go",
		Preamble: "package main\n\nimport (\n" + strings.Join(imports, "\n") + "\n)",
		Blocks: []assemblyline.SourceBlock{{ID: "application.run", Static: source,
			API:       "func RunApplication(arguments []string, standardInput string) string\nfunc main()",
			DependsOn: append([]string(nil), dependencies...)}},
	}, nil
}

func goCommandLineApplicationSource(requirements []assemblyline.Requirement, capabilities directCodingCapabilityGraph, kinds directCodingResultValueKindPlan, order []string, sources directCodingInputSourcePlan) (string, error) {
	indices := goValueIndices(requirements)
	invocations, _ := goValueInvocationLines(order, indices, capabilities, nil, sources)
	var source strings.Builder
	source.WriteString("func RunApplication(arguments []string, standardInput string) string {\n")
	for _, requirement := range requirements {
		sequence := indices[requirement.ID]
		expression := ""
		switch sources[requirement.ID] {
		case assemblyline.ApplicationInputArguments:
			expression = "arguments"
		case assemblyline.ApplicationInputStandardInput:
			expression = "standardInput"
		case assemblyline.ApplicationInputBoth:
			expression = "TaskInput{Arguments: arguments, StandardInput: standardInput}"
		case assemblyline.ApplicationInputNone:
			continue
		default:
			return "", fmt.Errorf("Go input channel is unresolved")
		}
		source.WriteString(fmt.Sprintf("input%03d := %s\n", sequence, expression))
	}
	source.WriteString(strings.Join(invocations, "\n") + "\n")
	source.WriteString("combined := \"\"\n")
	for _, id := range order {
		sequence := indices[id]
		formatted, err := goCommandLineFormatValue(kinds[id], fmt.Sprintf("value%03d", sequence))
		if err != nil {
			return "", err
		}
		source.WriteString(fmt.Sprintf("output%03d := %s\n", sequence, formatted))
		source.WriteString(fmt.Sprintf("if output%03d != \"\" { if combined != \"\" { combined += \"\\n\" }; combined += output%03d }\n", sequence, sequence))
	}
	source.WriteString("return combined\n}\n\n")
	source.WriteString("func main() {\nvar rawInput []byte\n")
	if goNeedsStandardInput(sources) {
		source.WriteString(`inputInfo, err := os.Stdin.Stat()
if err != nil { fmt.Fprintln(os.Stderr, "inspect standard input:", err); os.Exit(1) }
if inputInfo.Mode()&os.ModeCharDevice == 0 {
 rawInput, err = io.ReadAll(os.Stdin)
 if err != nil { fmt.Fprintln(os.Stderr, "read standard input:", err); os.Exit(1) }
}
`)
	}
	source.WriteString(`output := RunApplication(os.Args[1:], string(rawInput))
if output != "" { fmt.Fprintln(os.Stdout, output) }
}`)
	return source.String(), nil
}

func goCommandLineRequirementOrder(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
) ([]string, error) {
	nodes := make([]assemblyline.DependencyNode, 0, len(requirements))
	for _, requirement := range requirements {
		dependencies := make([]string, len(capabilities[requirement.ID]))
		for index, dependency := range capabilities[requirement.ID] {
			dependencies[index] = dependency.RequirementID
		}
		nodes = append(nodes, assemblyline.DependencyNode{ID: requirement.ID, DependsOn: dependencies})
	}
	waves, err := assemblyline.BuildDependencyWaves(nodes)
	if err != nil {
		return nil, fmt.Errorf("order Go command-line capability execution: %w", err)
	}
	order := make([]string, 0, len(requirements))
	for _, wave := range waves {
		order = append(order, wave...)
	}
	return order, nil
}

func goNeedsStandardInput(sources directCodingInputSourcePlan) bool {
	for _, source := range sources {
		if source == assemblyline.ApplicationInputStandardInput || source == assemblyline.ApplicationInputBoth {
			return true
		}
	}
	return false
}
