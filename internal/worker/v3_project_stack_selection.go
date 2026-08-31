package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingProjectSelection struct {
	Stack   directCodingProjectStack
	Profile directCodingProjectVersionProfile
	Dialect string
}

func selectDirectCodingProject(
	runtime typedWorkerRuntime,
	resolveModel func() (string, error),
	redactedRequest string,
	specification assemblyline.ApplicationSpecification,
	identities []assemblyline.ArtifactIdentity,
) (directCodingProjectSelection, error) {
	return selectDirectCodingProjectFromRegistries(
		runtime, resolveModel, redactedRequest, specification, identities,
		registeredDirectCodingProjectStacks(), registeredDirectCodingProjectVersionProfiles(),
	)
}

func selectDirectCodingProjectFromRegistries(
	runtime typedWorkerRuntime,
	resolveModel func() (string, error),
	redactedRequest string,
	specification assemblyline.ApplicationSpecification,
	identities []assemblyline.ArtifactIdentity,
	registeredStacks []directCodingProjectStack,
	registeredProfiles []directCodingProjectVersionProfile,
) (directCodingProjectSelection, error) {
	stacks := directCodingProjectStacksForSurfaceFrom(registeredStacks, specification.Surface)
	if len(stacks) == 0 {
		return directCodingProjectSelection{}, fmt.Errorf(
			"no registered project stack supports application surface %s",
			specification.Surface,
		)
	}
	formats, err := directCodingProjectFormatCandidates(stacks, registeredProfiles)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	if len(formats) == 1 {
		return directCodingProjectSelectionForFormat(formats[0])
	}
	input, err := directCodingProjectStackConstraintInput(redactedRequest, formats)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	if resolveModel == nil {
		return directCodingProjectSelection{}, fmt.Errorf(
			"project stack semantic selection requires one model resolver",
		)
	}
	modelName, err := resolveModel()
	if err != nil {
		return directCodingProjectSelection{}, err
	}

	job, err := assemblyline.NewApplicationProjectStackConstraintJob(input)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	decision, err := runDirectCodingSemanticLeafCall(
		runtime, modelName, "application_project_stack_constraint", job, identities,
		func(raw string) (assemblyline.ApplicationProjectStackConstraintDecision, error) {
			return assemblyline.DecodeApplicationProjectStackConstraintDecision(input, raw)
		},
	)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	return resolveDirectCodingProjectFormatDecision(
		formats, input, decision,
	)
}

func directCodingProjectStacksForSurface(
	surface assemblyline.ApplicationSurface,
) []directCodingProjectStack {
	return directCodingProjectStacksForSurfaceFrom(registeredDirectCodingProjectStacks(), surface)
}

func directCodingProjectStacksForSurfaceFrom(
	registered []directCodingProjectStack,
	surface assemblyline.ApplicationSurface,
) []directCodingProjectStack {
	stacks := make([]directCodingProjectStack, 0)
	for _, stack := range registered {
		if stack.SupportsSurface(surface) {
			stacks = append(stacks, stack)
		}
	}
	return stacks
}
