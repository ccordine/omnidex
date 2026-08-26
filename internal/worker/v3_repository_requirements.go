package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func interpretRepositoryRequirements(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	context assemblyline.ApplicationContext,
	identities []assemblyline.ArtifactIdentity,
) ([]string, error) {
	input := assemblyline.RepositoryRequirementInterpretationInput{
		UserRequest: authority, Context: context,
	}
	job, err := assemblyline.NewRepositoryRequirementInterpretationJob(input)
	if err != nil {
		return nil, err
	}
	oneCallRuntime := runtime
	oneCallRuntime.MaxAttempts = 1
	interpretation, err := runDirectCodingSemanticCall[assemblyline.RepositoryRequirementInterpretation](
		oneCallRuntime, modelName, "repository_requirements", job, identities,
		func(value assemblyline.RepositoryRequirementInterpretation) error {
			return value.ValidateFor(input)
		},
	)
	if err != nil {
		return nil, err
	}
	return assemblyline.ResolveRepositoryRequirements(input, interpretation)
}
