package worker

import "fmt"

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
