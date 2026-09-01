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
	dialect, err := directCodingProjectSourceDialect(profile)
	if err != nil {
		return "", err
	}
	format := strings.TrimSpace(stack.ConstraintDescription) +
		"; source dialect: " + dialect
	if len(versions) > 0 {
		format += "; qualified versions: " + strings.Join(versions, ", ")
	}
	for _, identity := range []string{stack.ID, profile.ID} {
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
	return input, nil
}

func resolveDirectCodingProjectFormatDecision(
	formats []directCodingProjectFormatCandidate,
	input assemblyline.ApplicationProjectStackConstraintInput,
	decision assemblyline.ApplicationProjectStackConstraintDecision,
) (directCodingProjectSelection, error) {
	if err := decision.ValidateFor(input); err != nil {
		return directCodingProjectSelection{}, err
	}
	switch decision.CandidateID {
	case assemblyline.ApplicationProjectStackUnconstrained:
		if len(formats) == 0 || len(formats) != len(input.Candidates) ||
			input.Candidates[0].TechnicalFormat != formats[0].Format {
			return directCodingProjectSelection{}, fmt.Errorf(
				"unconstrained project format lacks its code-owned default mapping",
			)
		}
		return directCodingProjectSelectionForFormat(formats[0])
	case assemblyline.ApplicationProjectStackUnsupported:
		return directCodingProjectSelection{}, fmt.Errorf(
			"accepted application authority requires an unsupported or contradictory technical format",
		)
	default:
		for index, candidate := range input.Candidates {
			if candidate.CandidateID == decision.CandidateID {
				if index >= len(formats) || candidate.TechnicalFormat != formats[index].Format {
					return directCodingProjectSelection{}, fmt.Errorf(
						"project format candidate %q lost its code-owned mapping", candidate.CandidateID,
					)
				}
				return directCodingProjectSelectionForFormat(formats[index])
			}
		}
		return directCodingProjectSelection{}, fmt.Errorf(
			"project format decision %q has no code-owned mapping", decision.CandidateID,
		)
	}
}

func directCodingProjectSelectionForFormat(
	format directCodingProjectFormatCandidate,
) (directCodingProjectSelection, error) {
	dialect, err := directCodingProjectSourceDialect(format.Profile)
	if err != nil {
		return directCodingProjectSelection{}, err
	}
	return directCodingProjectSelection{
		Stack: format.Stack, Profile: format.Profile, Dialect: dialect,
	}, nil
}
