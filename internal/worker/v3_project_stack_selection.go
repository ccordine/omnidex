package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingProjectSelection struct {
	Stack            directCodingProjectStack
	VersionProfileID string
}

func selectDirectCodingProject(
	runtime typedWorkerRuntime,
	resolveModel func() (string, error),
	specification assemblyline.ApplicationSpecification,
	existingManifests map[string]string,
	identities []assemblyline.ArtifactIdentity,
) (directCodingProjectSelection, error) {
	if err := validateDirectCodingArtifactRegistries(); err != nil {
		return directCodingProjectSelection{}, err
	}
	return selectDirectCodingProjectFromRegistries(
		runtime, resolveModel, specification, existingManifests, identities,
		registeredDirectCodingProjectStacks(), registeredDirectCodingProjectVersionProfiles(),
	)
}

func selectDirectCodingProjectFromRegistries(
	runtime typedWorkerRuntime,
	resolveModel func() (string, error),
	specification assemblyline.ApplicationSpecification,
	existingManifests map[string]string,
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
	profiles, err := directCodingProjectVersionProfilesForStacks(stacks, registeredProfiles)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	matchedProfiles, applicable, err := matchDirectCodingVersionProfiles(profiles, existingManifests)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	if len(matchedProfiles) > 1 {
		return directCodingProjectSelection{}, fmt.Errorf(
			"existing workspace surface %s matches %d registered version profiles",
			specification.Surface, len(matchedProfiles),
		)
	}
	if len(matchedProfiles) == 1 {
		return directCodingSelectionForMatchedProfile(stacks, matchedProfiles[0])
	}
	if applicable > 0 {
		return directCodingProjectSelection{}, fmt.Errorf(
			"existing workspace surface %s has no compatible registered version profile",
			specification.Surface,
		)
	}
	formats, err := directCodingProjectFormatCandidates(stacks, registeredProfiles)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	input, err := directCodingProjectStackConstraintInput(specification, formats)
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
		func(value assemblyline.ApplicationProjectStackConstraintDecision) error {
			return value.ValidateFor(input)
		},
	)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	return resolveDirectCodingProjectFormatDecision(
		specification.Surface, stacks, registeredProfiles, formats, input, decision,
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
