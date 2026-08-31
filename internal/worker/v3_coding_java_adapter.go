package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const genericJavaCommandLineAdapter = "java_command_line_capabilities_v1"

func compileGenericJavaCommandLineBlueprint(
	_ string,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	_ directCodingProjectVersionProfile,
	target assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error) {
	if err := validateJavaCommandLineCoverage(target, workload, coverage); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	contexts, err := directCodingApplicationTaskContexts(workload)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents, err := genericJavaCommandLineDocuments(
		specification, contexts, capabilities, coverage,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	blueprint := assemblyline.SourceBlueprint{Documents: documents}
	if err := assemblyline.ValidateJavaSourceBlueprint(blueprint); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	return blueprint, nil, nil
}

func validateJavaCommandLineCoverage(
	target assemblyline.TargetTree,
	workload assemblyline.FrozenApplicationWorkload,
	coverage assemblyline.ApplicationFileCoveragePlan,
) error {
	for _, file := range coverage.Files {
		if len(file.TaskIDs) != 1 {
			return fmt.Errorf(
				"Java command-line source %s requires exactly one task owner", file.Path,
			)
		}
	}
	for _, task := range workload.Tasks {
		if _, err := directCodingTaskSingleImplementationPath(coverage, task.ID); err != nil {
			return fmt.Errorf("validate Java command-line task %s coverage: %w", task.ID, err)
		}
	}
	return nil
}
