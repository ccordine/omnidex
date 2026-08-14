package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func interpretRepositoryRequirements(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) ([]string, error) {
	input := assemblyline.RepositoryRequirementInterpretationInput{UserRequest: authority}
	job, err := assemblyline.NewRepositoryRequirementInterpretationJob(input)
	if err != nil {
		return nil, err
	}
	oneCallRuntime := runtime
	oneCallRuntime.MaxAttempts = 1
	interpretation, err := runDirectCodingSemanticCall[assemblyline.RepositoryRequirementInterpretation](
		oneCallRuntime, modelName, "repository_requirements", job, identities,
		func(value assemblyline.RepositoryRequirementInterpretation) error {
			_, validationErr := assemblyline.ResolveRepositoryRequirements(input, value)
			return validationErr
		},
	)
	if err != nil {
		return nil, err
	}
	return assemblyline.ResolveRepositoryRequirements(input, interpretation)
}
