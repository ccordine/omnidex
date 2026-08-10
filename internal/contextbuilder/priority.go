package contextbuilder

import (
	"sort"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func roleRank(role workingset.Role) int {
	switch role {
	case workingset.RoleUserAuthority:
		return 1
	case workingset.RoleGoal:
		return 2
	case workingset.RoleObjective:
		return 3
	case workingset.RoleTask:
		return 4
	case workingset.RoleAcceptanceCriterion:
		return 5
	case workingset.RoleConstraint:
		return 6
	case workingset.RoleFact:
		return 7
	case workingset.RoleHypothesis:
		return 8
	case workingset.RoleDecision:
		return 9
	case workingset.RoleInvariant:
		return 10
	case workingset.RoleFailure:
		return 11
	case workingset.RoleQuestion:
		return 12
	case workingset.RoleEvidence:
		return 13
	case workingset.RoleRepositoryEvidence:
		return 14
	case workingset.RoleDependency:
		return 15
	case workingset.RoleVerification:
		return 16
	case workingset.RoleHistorical:
		return 17
	default:
		return 0
	}
}

func authorityRank(authority taskstate.Authority) int {
	switch authority {
	case taskstate.AuthorityUser:
		return 1
	case taskstate.AuthorityCode:
		return 2
	case taskstate.AuthorityToolEvidence:
		return 3
	case taskstate.AuthorityAcceptedModelDecision:
		return 4
	case taskstate.AuthorityModelProposal:
		return 5
	default:
		return 0
	}
}

func sortedSelectors(selectors []Selector) []Selector {
	result := append([]Selector(nil), selectors...)
	sort.Slice(result, func(left, right int) bool {
		leftRank, rightRank := roleRank(result[left].Role), roleRank(result[right].Role)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return result[left].ID < result[right].ID
	})
	return result
}

func sortedItems(items []workingset.Item) []workingset.Item {
	result := append([]workingset.Item(nil), items...)
	sort.Slice(result, func(left, right int) bool {
		leftRank, rightRank := roleRank(result[left].Role), roleRank(result[right].Role)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if result[left].Priority != result[right].Priority {
			return result[left].Priority > result[right].Priority
		}
		if result[left].LastUsedTick != result[right].LastUsedTick {
			return result[left].LastUsedTick > result[right].LastUsedTick
		}
		return result[left].ID < result[right].ID
	})
	return result
}
