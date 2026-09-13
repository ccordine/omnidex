package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingResultValueKindPlan map[string]assemblyline.ApplicationResultValueKind

func resolveDirectCodingResultValueKinds(runtime typedWorkerRuntime, model string, workload assemblyline.FrozenApplicationWorkload, identities []assemblyline.ArtifactIdentity) (directCodingResultValueKindPlan, error) {
	kinds := make(directCodingResultValueKindPlan, len(workload.Tasks))
	for _, task := range workload.Tasks {
		input := assemblyline.ApplicationResultValueKindInput{Requirement: task.RequirementQuote}
		job, err := assemblyline.NewApplicationResultValueKindJob(input)
		if err != nil {
			return nil, err
		}
		kind, err := runDirectCodingSemanticLeafCall(runtime, model, "application_result_value_kind", job, identities,
			func(raw string) (assemblyline.ApplicationResultValueKind, error) {
				return assemblyline.DecodeApplicationResultValueKind(input, raw)
			})
		if err != nil {
			return nil, err
		}
		if _, err := goCommandLineValueType(kind); err != nil {
			return nil, fmt.Errorf("requirement %s: %w", task.RequirementID, err)
		}
		kinds[task.RequirementID] = kind
	}
	return kinds, nil
}

func goCommandLineValueType(kind assemblyline.ApplicationResultValueKind) (string, error) {
	switch kind {
	case assemblyline.ApplicationResultText:
		return "string", nil
	case assemblyline.ApplicationResultInteger:
		return "int", nil
	case assemblyline.ApplicationResultDecimal:
		return "float64", nil
	case assemblyline.ApplicationResultBoolean:
		return "bool", nil
	default:
		return "", fmt.Errorf("Go value adapter requires one supported text, integer, decimal, or Boolean result; received %q", kind)
	}
}

func goCommandLineFormatValue(kind assemblyline.ApplicationResultValueKind, value string) (string, error) {
	switch kind {
	case assemblyline.ApplicationResultText:
		return value, nil
	case assemblyline.ApplicationResultInteger:
		return "strconv.Itoa(" + value + ")", nil
	case assemblyline.ApplicationResultDecimal:
		return "strconv.FormatFloat(" + value + ", 'g', -1, 64)", nil
	case assemblyline.ApplicationResultBoolean:
		return "strconv.FormatBool(" + value + ")", nil
	default:
		return "", fmt.Errorf("unsupported Go result formatting kind %q", kind)
	}
}
