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
	return runDirectCodingSemanticLeafCall(
		runtime,
		modelName,
		"declaration_artifact_boundary",
		job,
		nil,
		func(raw string) (assemblyline.DeclarationArtifactBoundaryDecision, error) {
			return assemblyline.DecodeDeclarationArtifactBoundaryDecision(input, raw)
		},
		func(value assemblyline.DeclarationArtifactBoundaryDecision) error {
			return value.ValidateFor(input)
		},
	)
}
