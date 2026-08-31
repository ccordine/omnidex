package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingVersionProfilesForStack(
	stack directCodingProjectStack,
) ([]directCodingProjectVersionProfile, error) {
	return directCodingVersionProfilesForStackFrom(
		registeredDirectCodingProjectVersionProfiles(), stack,
	)
}

func directCodingVersionProfilesForStackFrom(
	registered []directCodingProjectVersionProfile,
	stack directCodingProjectStack,
) ([]directCodingProjectVersionProfile, error) {
	profiles := make([]directCodingProjectVersionProfile, 0, 1)
	for _, profile := range registered {
		if profile.StackID == stack.ID {
			profiles = append(profiles, cloneDirectCodingProjectVersionProfile(profile))
		}
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("project stack %s has no registered version profiles", stack.ID)
	}
	return profiles, nil
}

func directCodingDefaultVersionProfileForStack(
	stack directCodingProjectStack,
) (directCodingProjectVersionProfile, error) {
	profile, err := directCodingProjectVersionProfileByID(stack.DefaultVersionProfileID)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	if profile.StackID != stack.ID {
		return directCodingProjectVersionProfile{}, fmt.Errorf(
			"project stack %s default version profile %s qualifies stack %s",
			stack.ID, profile.ID, profile.StackID,
		)
	}
	return profile, nil
}

func directCodingVersionProfileForTargetTree(
	target assemblyline.TargetTree,
) (directCodingProjectVersionProfile, error) {
	stack, err := directCodingProjectStackByID(target.StackID)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	profile, err := directCodingProjectVersionProfileByID(target.VersionProfileID)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	if profile.StackID != stack.ID {
		return directCodingProjectVersionProfile{}, fmt.Errorf(
			"target tree stack %s is not qualified by version profile %s", stack.ID, profile.ID,
		)
	}
	return profile, nil
}

func directCodingVersionProfileForProgram(
	program directCodingProgram,
) (directCodingProjectVersionProfile, error) {
	if err := validateDirectCodingProgramTargetTreeAuthority(program); err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	profile, err := directCodingProjectVersionProfileByID(program.VersionProfileID)
	if err != nil {
		return directCodingProjectVersionProfile{}, err
	}
	if profile.StackID != stack.ID {
		return directCodingProjectVersionProfile{}, fmt.Errorf(
			"program stack %s is not qualified by version profile %s",
			stack.ID, profile.ID,
		)
	}
	return profile, nil
}

func validateDirectCodingProgramTargetTreeAuthority(program directCodingProgram) error {
	target := program.TargetTree
	if len(target.Paths) == 0 && target.StackID == "" && target.VersionProfileID == "" {
		return nil
	}
	if target.StackID != program.StackID {
		return fmt.Errorf(
			"program target tree stack %q differs from program authority %q",
			target.StackID, program.StackID,
		)
	}
	if target.VersionProfileID != program.VersionProfileID {
		return fmt.Errorf(
			"program target tree version profile %q differs from program authority %q",
			target.VersionProfileID, program.VersionProfileID,
		)
	}
	return nil
}
