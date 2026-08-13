package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func classifyDeclarationArtifactBoundary(
	runtime typedWorkerRuntime,
	modelName string,
	input assemblyline.DeclarationArtifactBoundaryInput,
) (assemblyline.DeclarationArtifactBoundaryDecision, error) {
	job, err := assemblyline.NewDeclarationArtifactBoundaryJob(input)
	if err != nil {
		return assemblyline.DeclarationArtifactBoundaryDecision{}, err
	}
	return runDirectCodingSemanticCall[assemblyline.DeclarationArtifactBoundaryDecision](
		runtime,
		modelName,
		"declaration_artifact_boundary",
		job,
		nil,
		func(value assemblyline.DeclarationArtifactBoundaryDecision) error {
			return value.ValidateFor(input)
		},
	)
}
