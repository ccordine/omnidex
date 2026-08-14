package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

const (
	maxRepositoryGroundedAdvisoryCapsules = 1
	maxRepositoryGroundedAdvisoryIDBytes  = 256
)

func validateRepositoryGroundedAdvisoryCapsules(
	capsules []objectiveadvisory.Capsule,
	evidenceIDs map[string]struct{},
) error {
	if len(capsules) > maxRepositoryGroundedAdvisoryCapsules {
		return fmt.Errorf(
			"repository grounded review accepts at most %d selected advisory capsule",
			maxRepositoryGroundedAdvisoryCapsules,
		)
	}
	for index, capsule := range capsules {
		if err := validateRepositoryGroundedAdvisoryCapsule(capsule); err != nil {
			return fmt.Errorf("repository grounded review advisory capsule %d: %w", index, err)
		}
		if _, overlaps := evidenceIDs[capsule.ID]; overlaps {
			return fmt.Errorf(
				"repository grounded review advisory capsule %q cannot also be cited evidence",
				capsule.ID,
			)
		}
	}
	return nil
}

func validateRepositoryGroundedAdvisoryCapsule(capsule objectiveadvisory.Capsule) error {
	if err := validateGroundedID(
		"advisory objective ID", capsule.ObjectiveID,
		maxRepositoryGroundedAdvisoryIDBytes,
	); err != nil {
		return err
	}
	if capsule.Generation < 1 {
		return fmt.Errorf("advisory capsule requires a positive objective generation")
	}
	return capsule.ValidateFor(capsule.ObjectiveID, capsule.Generation)
}
