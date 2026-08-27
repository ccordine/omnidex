package contextcompiler

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type optionalSelectionGroupMember struct {
	index     int
	namespace string
}

func validateOptionalSelectionGroups(set CandidateSet) error {
	if len(set.OptionalSelectionGroups) == 0 {
		return nil
	}
	requiredIDs := make(map[string]struct{}, len(set.Required))
	for _, authority := range set.Required {
		requiredIDs[authority.CandidateID] = struct{}{}
	}
	optionalMembers := make(map[string]optionalSelectionGroupMember, len(set.Optional))
	for index, authority := range set.Optional {
		optionalMembers[authority.CandidateID] = optionalSelectionGroupMember{
			index: index, namespace: authority.Namespace,
		}
	}
	groupedIDs := make(map[string]int)
	for groupIndex, group := range set.OptionalSelectionGroups {
		if len(group.CandidateIDs) < 2 {
			return fmt.Errorf(
				"optional selection group %d requires at least two candidate IDs",
				groupIndex,
			)
		}
		seenInGroup := make(map[string]struct{}, len(group.CandidateIDs))
		previousIndex := -1
		namespace := ""
		for memberIndex, candidateID := range group.CandidateIDs {
			if _, duplicate := seenInGroup[candidateID]; duplicate {
				return fmt.Errorf(
					"optional selection group %d duplicates candidate ID %q",
					groupIndex, candidateID,
				)
			}
			seenInGroup[candidateID] = struct{}{}
			if previousGroup, overlap := groupedIDs[candidateID]; overlap {
				return fmt.Errorf(
					"optional selection groups %d and %d overlap at candidate ID %q",
					previousGroup, groupIndex, candidateID,
				)
			}
			if _, required := requiredIDs[candidateID]; required {
				return fmt.Errorf(
					"optional selection group %d contains required candidate ID %q",
					groupIndex, candidateID,
				)
			}
			member, exists := optionalMembers[candidateID]
			if !exists {
				return fmt.Errorf(
					"optional selection group %d contains unknown candidate ID %q",
					groupIndex, candidateID,
				)
			}
			if memberIndex == 0 {
				namespace = member.namespace
			} else {
				if member.namespace != namespace {
					return fmt.Errorf(
						"optional selection group %d crosses candidate namespaces",
						groupIndex,
					)
				}
				if member.index != previousIndex+1 {
					return fmt.Errorf(
						"optional selection group %d is not contiguous in optional authority order",
						groupIndex,
					)
				}
			}
			previousIndex = member.index
		}
		for _, candidateID := range group.CandidateIDs {
			groupedIDs[candidateID] = groupIndex
		}
	}
	return nil
}

func expandOptionalSelectionGroups(
	authorities []assemblyline.ContextCandidateAuthority,
	selected []assemblyline.ContextCandidateAuthority,
	groups []OptionalSelectionGroup,
) []assemblyline.ContextCandidateAuthority {
	selectedIDs := make(map[string]struct{}, len(selected))
	for _, authority := range selected {
		selectedIDs[authority.CandidateID] = struct{}{}
	}
	for _, group := range groups {
		expand := false
		for _, candidateID := range group.CandidateIDs {
			if _, exists := selectedIDs[candidateID]; exists {
				expand = true
				break
			}
		}
		if !expand {
			continue
		}
		for _, candidateID := range group.CandidateIDs {
			selectedIDs[candidateID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(selectedIDs))
	for candidateID := range selectedIDs {
		ids = append(ids, candidateID)
	}
	return selectedInAuthorityOrder(authorities, ids)
}
