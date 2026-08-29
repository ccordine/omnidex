package worker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var directCodingRuntimeCapabilityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

func validateDirectCodingRuntimeCapabilityRegistry(
	candidates []directCodingRuntimeCapability,
) error {
	if len(candidates) < 1 || len(candidates) > maxDirectCodingRuntimeCapabilitiesPerRequirement {
		return fmt.Errorf(
			"runtime capability registry requires between 1 and %d entries",
			maxDirectCodingRuntimeCapabilitiesPerRequirement,
		)
	}
	seenIDs := make(map[string]struct{}, len(candidates))
	seenPurposes := make(map[string]struct{}, len(candidates))
	lastID := ""
	for index, candidate := range candidates {
		if !directCodingRuntimeCapabilityIDPattern.MatchString(candidate.ID) ||
			candidate.ID != strings.TrimSpace(candidate.ID) {
			return fmt.Errorf("runtime capability %d has invalid ID %q", index, candidate.ID)
		}
		if lastID != "" && candidate.ID <= lastID {
			return fmt.Errorf("runtime capability registry is not ordered by ID")
		}
		lastID = candidate.ID
		if _, duplicate := seenIDs[candidate.ID]; duplicate {
			return fmt.Errorf("runtime capability ID %q is registered more than once", candidate.ID)
		}
		seenIDs[candidate.ID] = struct{}{}
		if candidate.Purpose == "" || candidate.Purpose != strings.TrimSpace(candidate.Purpose) ||
			len(candidate.Purpose) > 512 || strings.ContainsAny(candidate.Purpose, "\x00\r\n") {
			return fmt.Errorf("runtime capability %s has invalid semantic purpose", candidate.ID)
		}
		if _, duplicate := seenPurposes[candidate.Purpose]; duplicate {
			return fmt.Errorf("runtime capability %s repeats a semantic purpose", candidate.ID)
		}
		seenPurposes[candidate.Purpose] = struct{}{}
		if err := assemblyline.ValidatePathFreeModelContext(
			"runtime capability purpose", candidate.Purpose,
		); err != nil {
			return fmt.Errorf("runtime capability %s purpose: %w", candidate.ID, err)
		}
	}
	return nil
}

func validateDirectCodingRuntimeCapabilityGraph(
	requirements []assemblyline.Requirement,
	candidates []directCodingRuntimeCapability,
	graph directCodingRuntimeCapabilityGraph,
) error {
	if err := validateDirectCodingRuntimeCapabilityRegistry(candidates); err != nil {
		return err
	}
	if len(graph) != len(requirements) {
		return fmt.Errorf(
			"runtime capability graph=%d does not cover requirements=%d",
			len(graph), len(requirements),
		)
	}
	indices := make(map[string]int, len(candidates))
	for index, candidate := range candidates {
		indices[candidate.ID] = index
	}
	for _, requirement := range requirements {
		selected, exists := graph[requirement.ID]
		if !exists {
			return fmt.Errorf("runtime capability graph omits requirement %s", requirement.ID)
		}
		if len(selected) > maxDirectCodingRuntimeCapabilitiesPerRequirement {
			return fmt.Errorf(
				"requirement %s exceeds the %d runtime-capability bound",
				requirement.ID, maxDirectCodingRuntimeCapabilitiesPerRequirement,
			)
		}
		lastIndex := -1
		for _, capabilityID := range selected {
			index, registered := indices[capabilityID]
			if !registered {
				return fmt.Errorf(
					"requirement %s names unregistered runtime capability %s",
					requirement.ID, capabilityID,
				)
			}
			if index <= lastIndex {
				return fmt.Errorf(
					"requirement %s runtime capabilities are duplicated or unordered",
					requirement.ID,
				)
			}
			lastIndex = index
		}
	}
	return nil
}
