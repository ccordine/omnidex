package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingProjectFormatCandidate struct {
	Stack   directCodingProjectStack
	Profile directCodingProjectVersionProfile
	Format  string
}

func directCodingProjectVersionProfilesForStacks(
	stacks []directCodingProjectStack,
	profiles []directCodingProjectVersionProfile,
) ([]directCodingProjectVersionProfile, error) {
	selected := make([]directCodingProjectVersionProfile, 0, len(stacks))
	for _, stack := range stacks {
		stackProfiles, err := directCodingVersionProfilesForStackFrom(profiles, stack)
		if err != nil {
			return nil, err
		}
		selected = append(selected, stackProfiles...)
	}
	return selected, nil
}

func directCodingProjectFormatCandidates(
	stacks []directCodingProjectStack,
	profiles []directCodingProjectVersionProfile,
) ([]directCodingProjectFormatCandidate, error) {
	candidates := make([]directCodingProjectFormatCandidate, 0, len(stacks))
	for _, stack := range stacks {
		stackProfiles, err := directCodingVersionProfilesForStackFrom(profiles, stack)
		if err != nil {
			return nil, err
		}
		for _, profile := range stackProfiles {
			format, err := directCodingProjectVersionTechnicalFormat(stack, profile)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, directCodingProjectFormatCandidate{
				Stack: stack, Profile: profile, Format: format,
			})
		}
	}
	return candidates, nil
}

func directCodingProjectVersionTechnicalFormat(
	stack directCodingProjectStack,
	profile directCodingProjectVersionProfile,
) (string, error) {
	if profile.StackID != stack.ID {
		return "", fmt.Errorf(
			"version profile %s qualifies stack %s, not candidate stack %s",
			profile.ID, profile.StackID, stack.ID,
		)
	}
	components := append([]directCodingProjectVersionComponent(nil), profile.Components...)
	sort.Slice(components, func(left, right int) bool {
		if components[left].Name != components[right].Name {
			return components[left].Name < components[right].Name
		}
		return components[left].Version < components[right].Version
	})
	versions := make([]string, len(components))
	for index, component := range components {
		name := strings.Join(strings.Fields(strings.ReplaceAll(component.Name, "_", " ")), " ")
		version := strings.TrimSpace(component.Version)
		if name == "" || version == "" || component.Name != strings.TrimSpace(component.Name) ||
			component.Version != version || strings.ContainsAny(name+version, "\x00\r\n") {
			return "", fmt.Errorf("version profile %s has an invalid technical component", profile.ID)
		}
		versions[index] = name + " " + version
	}
	packagingShape := strings.TrimSpace(stack.TreeDescription)
	if packagingShape == "" || stack.TreeDescription != packagingShape ||
		strings.ContainsAny(packagingShape, "\x00\r\n") {
		return "", fmt.Errorf("project stack %s has an invalid packaging shape", stack.ID)
	}
	format := strings.TrimSpace(stack.ConstraintDescription) +
		"; packaging shape: " + packagingShape +
		"; source dialect: " + strings.TrimSpace(profile.SourceDialect)
	if len(versions) > 0 {
		format += "; qualified versions: " + strings.Join(versions, ", ")
	}
	for _, identity := range append(
		[]string{stack.ID, profile.ID, profile.ParserQualification},
		profile.ManifestPaths...,
	) {
		if identity != "" && strings.Contains(format, identity) {
			return "", fmt.Errorf("project technical format exposes internal identity %q", identity)
		}
	}
	if err := assemblyline.ValidatePathFreeModelContext("project technical format", format); err != nil {
		return "", err
	}
	return format, nil
}

func directCodingProjectStackConstraintInput(
	redactedRequest string,
	formats []directCodingProjectFormatCandidate,
) (assemblyline.ApplicationProjectStackConstraintInput, error) {
	candidates := make([]assemblyline.ApplicationProjectStackCandidate, len(formats))
	for index, format := range formats {
		candidates[index] = assemblyline.ApplicationProjectStackCandidate{
			CandidateID:     fmt.Sprintf("STACK_CANDIDATE_%d", index+1),
			TechnicalFormat: format.Format,
		}
	}
	input := assemblyline.ApplicationProjectStackConstraintInput{
		UserRequest: redactedRequest, Candidates: candidates,
	}
	if _, err := assemblyline.NewApplicationProjectStackConstraintJob(input); err != nil {
		return assemblyline.ApplicationProjectStackConstraintInput{}, err
	}
	return input, nil
}

func resolveDirectCodingProjectFormatDecision(
	surface assemblyline.ApplicationSurface,
	stacks []directCodingProjectStack,
	profiles []directCodingProjectVersionProfile,
	formats []directCodingProjectFormatCandidate,
	input assemblyline.ApplicationProjectStackConstraintInput,
	decision assemblyline.ApplicationProjectStackConstraintDecision,
) (directCodingProjectSelection, error) {
	if err := decision.ValidateFor(input); err != nil {
		return directCodingProjectSelection{}, err
	}
	switch decision.CandidateID {
	case assemblyline.ApplicationProjectStackUnsupported:
		return directCodingProjectSelection{}, fmt.Errorf(
			"accepted application authority requires an unsupported or contradictory technical format for surface %s",
			surface,
		)
	case assemblyline.ApplicationProjectStackUnconstrained:
		return directCodingDefaultProjectSelection(surface, stacks, profiles)
	default:
		for index, candidate := range input.Candidates {
			if candidate.CandidateID == decision.CandidateID {
				if index >= len(formats) || candidate.TechnicalFormat != formats[index].Format {
					return directCodingProjectSelection{}, fmt.Errorf(
						"project format candidate %q lost its code-owned mapping", candidate.CandidateID,
					)
				}
				selection := directCodingProjectSelection{
					Stack: formats[index].Stack, VersionProfileID: formats[index].Profile.ID,
				}
				return selection, nil
			}
		}
		return directCodingProjectSelection{}, fmt.Errorf(
			"project format decision %q has no code-owned mapping", decision.CandidateID,
		)
	}
}

func directCodingDefaultProjectSelection(
	surface assemblyline.ApplicationSurface,
	stacks []directCodingProjectStack,
	profiles []directCodingProjectVersionProfile,
) (directCodingProjectSelection, error) {
	var selected *directCodingProjectStack
	for index := range stacks {
		if !stacks[index].IsDefaultForSurface(surface) {
			continue
		}
		if selected != nil {
			return directCodingProjectSelection{}, fmt.Errorf(
				"surface %s has multiple default project stacks", surface,
			)
		}
		selected = &stacks[index]
	}
	if selected == nil {
		return directCodingProjectSelection{}, fmt.Errorf("surface %s has no default project stack", surface)
	}
	var defaultProfile *directCodingProjectVersionProfile
	for index := range profiles {
		if profiles[index].ID != selected.DefaultVersionProfileID {
			continue
		}
		if defaultProfile != nil {
			return directCodingProjectSelection{}, fmt.Errorf(
				"project stack %s has multiple default version profiles", selected.ID,
			)
		}
		profile := cloneDirectCodingProjectVersionProfile(profiles[index])
		defaultProfile = &profile
	}
	if defaultProfile == nil || defaultProfile.StackID != selected.ID {
		return directCodingProjectSelection{}, fmt.Errorf(
			"project stack %s has no registered matching default version profile", selected.ID,
		)
	}
	return directCodingProjectSelection{
		Stack: *selected, VersionProfileID: defaultProfile.ID,
	}, nil
}
