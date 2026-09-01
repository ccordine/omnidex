package worker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func goCommandLineRuntimeDocument() assemblyline.SourceDocument {
	const source = `type TaskInput struct {
	Arguments     []string // Command-line arguments excluding the executable name.
	StandardInput string   // Complete standard-input text.
}

type TaskResult struct {
	Output   string            // User-visible standard-output text.
	Error    string            // User-visible standard-error text.
	ExitCode int               // Process status; zero means success.
	State    map[string]string // Reusable capability values.
}

type CapabilityResults map[string]TaskResult`
	return assemblyline.SourceDocument{
		ID: "application_runtime", Path: "runtime.go", Preamble: "package main",
		Blocks: []assemblyline.SourceBlock{{
			ID: "runtime.api", Static: source, API: source,
		}},
	}
}

func goCommandLineApplicationDocument(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	order []string,
	dependencies []string,
) assemblyline.SourceDocument {
	return assemblyline.SourceDocument{
		ID: "application_entrypoint", Path: "main.go",
		Preamble: "package main\n\nimport (\n\t\"fmt\"\n\t\"io\"\n\t\"os\"\n)",
		Blocks: []assemblyline.SourceBlock{{
			ID:        "application.run",
			Static:    goCommandLineApplicationSource(requirements, capabilities, order),
			API:       "func RunApplication(arguments []string, standardInput string) TaskResult\nfunc main()",
			DependsOn: append([]string(nil), dependencies...),
		}},
	}
}

func goCommandLineApplicationSource(
	requirements []assemblyline.Requirement,
	capabilities directCodingCapabilityGraph,
	order []string,
) string {
	indices := make(map[string]int, len(requirements))
	for index, requirement := range requirements {
		indices[requirement.ID] = index + 1
	}
	var source strings.Builder
	source.WriteString("func RunApplication(arguments []string, standardInput string) TaskResult {\n")
	source.WriteString("\tinput := TaskInput{Arguments: arguments, StandardInput: standardInput}\n")
	source.WriteString("\tresults := CapabilityResults{}\n")
	source.WriteString("\tcombined := TaskResult{State: map[string]string{}}\n")
	for _, requirementID := range order {
		sequence := indices[requirementID]
		source.WriteString(fmt.Sprintf("\tdirect%03d := CapabilityResults{\n", sequence))
		for _, dependency := range capabilities[requirementID] {
			dependencySequence := indices[dependency.RequirementID]
			constant := fmt.Sprintf("Feature%03dCapability%03d", sequence, dependencySequence)
			source.WriteString(fmt.Sprintf(
				"\t\t%s: results[%s],\n", constant, strconv.Quote(dependency.CapabilityID),
			))
		}
		source.WriteString("\t}\n")
		source.WriteString(fmt.Sprintf(
			"\tresult%03d := Feature%03d(input, direct%03d)\n", sequence, sequence, sequence,
		))
		source.WriteString(fmt.Sprintf(
			"\tresults[%s] = result%03d\n", strconv.Quote(genericApplicationCapabilityID(sequence)), sequence,
		))
		source.WriteString(fmt.Sprintf("\tif result%03d.Output != \"\" {\n", sequence))
		source.WriteString("\t\tif combined.Output != \"\" { combined.Output += \"\\n\" }\n")
		source.WriteString(fmt.Sprintf("\t\tcombined.Output += result%03d.Output\n\t}\n", sequence))
		source.WriteString(fmt.Sprintf("\tfor key, value := range result%03d.State { combined.State[key] = value }\n", sequence))
		source.WriteString(fmt.Sprintf("\tif result%03d.ExitCode != 0 { combined.ExitCode = result%03d.ExitCode }\n", sequence, sequence))
		source.WriteString(fmt.Sprintf("\tif result%03d.Error != \"\" {\n", sequence))
		source.WriteString(fmt.Sprintf("\t\tcombined.Error = result%03d.Error\n", sequence))
		source.WriteString("\t\tif combined.ExitCode == 0 { combined.ExitCode = 1 }\n\t\treturn combined\n\t}\n")
	}
	source.WriteString("\treturn combined\n}\n\n")
	source.WriteString(`func main() {
	rawInput, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read standard input:", err)
		os.Exit(1)
	}
	result := RunApplication(os.Args[1:], string(rawInput))
	if result.Output != "" { fmt.Fprintln(os.Stdout, result.Output) }
	if result.Error != "" { fmt.Fprintln(os.Stderr, result.Error) }
	if result.ExitCode != 0 { os.Exit(result.ExitCode) }
}`)
	return source.String()
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
