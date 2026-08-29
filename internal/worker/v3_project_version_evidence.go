package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func snapshotDirectCodingVersionManifests(
	root string,
	existingPaths []string,
) (map[string]string, error) {
	present := make(map[string]struct{}, len(existingPaths))
	for index, value := range existingPaths {
		normalized, err := normalizeDirectCodingPath(value)
		if err != nil || normalized != value {
			return nil, fmt.Errorf("version evidence path %d is not exactly normalized", index)
		}
		if _, duplicate := present[value]; duplicate {
			return nil, fmt.Errorf("version evidence path %d duplicates %q", index, value)
		}
		present[value] = struct{}{}
	}
	manifestSet := make(map[string]struct{})
	for _, profile := range registeredDirectCodingProjectVersionProfiles() {
		for _, manifestPath := range profile.ManifestPaths {
			manifestSet[manifestPath] = struct{}{}
		}
	}
	manifestPaths := make([]string, 0, len(manifestSet))
	for manifestPath := range manifestSet {
		if _, exists := present[manifestPath]; exists {
			manifestPaths = append(manifestPaths, manifestPath)
		}
	}
	sort.Strings(manifestPaths)
	manifests := make(map[string]string, len(manifestPaths))
	for _, manifestPath := range manifestPaths {
		source, err := directCodingTargetTreeExistingSource(root, manifestPath)
		if err != nil {
			return nil, fmt.Errorf("read version evidence %s: %w", manifestPath, err)
		}
		manifests[manifestPath] = source
	}
	return manifests, nil
}

func matchDirectCodingVersionProfiles(
	profiles []directCodingProjectVersionProfile,
	manifests map[string]string,
) ([]directCodingProjectVersionProfile, int, error) {
	compatible := make([]directCodingProjectVersionProfile, 0, 1)
	applicable := 0
	for _, profile := range profiles {
		if profile.MatchExisting == nil {
			return nil, 0, fmt.Errorf("project version profile %s has no compatibility matcher", profile.ID)
		}
		status, err := profile.MatchExisting(profile, manifests)
		if err != nil {
			return nil, 0, fmt.Errorf("match project version profile %s: %w", profile.ID, err)
		}
		switch status {
		case directCodingVersionNotApplicable:
		case directCodingVersionCompatible:
			applicable++
			compatible = append(compatible, profile)
		case directCodingVersionUnsupported:
			applicable++
		default:
			return nil, 0, fmt.Errorf(
				"project version profile %s returned unsupported compatibility %q", profile.ID, status,
			)
		}
	}
	return compatible, applicable, nil
}

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
