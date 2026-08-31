package worker

import (
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const genericJavaCommandLineAdapter = "java_command_line_capabilities_v1"

func compileGenericJavaCommandLineBlueprint(
	_ string,
	specification assemblyline.ApplicationSpecification,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	target assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error) {
	if err := specification.Validate(); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if specification.Surface != assemblyline.ApplicationSurfaceCommandLine {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"generic Java command-line stack does not support surface %s", specification.Surface,
		)
	}
	if target.StackID != genericJavaCommandLineAdapter {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"Java command-line compiler received target stack %q", target.StackID,
		)
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
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
	if err := coverage.ValidateFor(target, workload); err != nil {
		return fmt.Errorf("validate Java command-line file coverage: %w", err)
	}
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

func validateJavaCommandLineAssembly(assembly directCodingAssembly) error {
	files := make(map[string]string, len(assembly.Files))
	for _, file := range assembly.Files {
		files[file.Path] = string(file.Content)
		if strings.HasSuffix(file.Path, ".java") && path.Dir(file.Path) != "." {
			return fmt.Errorf("Java command-line source %s must belong to the root package", file.Path)
		}
	}
	for _, required := range []string{"Main.java", "Runtime.java"} {
		if _, exists := files[required]; !exists {
			return fmt.Errorf("Java command-line assembly lacks code-owned artifact %s", required)
		}
	}
	return nil
}
