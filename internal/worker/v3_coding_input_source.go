package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingInputSourcePlan map[string]assemblyline.ApplicationInputSource

func resolveDirectCodingInputSources(runtime typedWorkerRuntime, model string, workload assemblyline.FrozenApplicationWorkload, identities []assemblyline.ArtifactIdentity) (directCodingInputSourcePlan, error) {
	sources := make(directCodingInputSourcePlan, len(workload.Tasks))
	for _, task := range workload.Tasks {
		input := assemblyline.ApplicationInputSourceInput{Requirement: task.RequirementQuote}
		job, err := assemblyline.NewApplicationInputSourceJob(input)
		if err != nil {
			return nil, err
		}
		source, err := runDirectCodingSemanticLeafCall(runtime, model, "application_input_source", job, identities,
			func(raw string) (assemblyline.ApplicationInputSource, error) {
				return assemblyline.DecodeApplicationInputSource(input, raw)
			})
		if err != nil {
			return nil, err
		}
		sources[task.RequirementID] = source
	}
	return sources, nil
}

func goInputParameter(source assemblyline.ApplicationInputSource) (string, string, error) {
	switch source {
	case assemblyline.ApplicationInputArguments:
		return "arguments", "[]string", nil
	case assemblyline.ApplicationInputStandardInput:
		return "text", "string", nil
	case assemblyline.ApplicationInputBoth:
		return "input", "TaskInput", nil
	case assemblyline.ApplicationInputNone:
		return "", "struct{}", nil
	default:
		return "", "", fmt.Errorf("Go input channel is unresolved: %q", source)
	}
}

func goInputCapabilities(source assemblyline.ApplicationInputSource) []string {
	if source == assemblyline.ApplicationInputBoth {
		return []string{"runtime.api"}
	}
	return nil
}
