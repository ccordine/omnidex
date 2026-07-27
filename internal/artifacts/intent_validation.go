package artifacts

import (
	"fmt"
	"sort"
	"strings"
)

const maxIntentAcceptanceCriteria = 12

func (a IntentArtifact) Validate() error {
	violations := make([]string, 0, 8)
	if strings.TrimSpace(a.UserGoal) == "" {
		violations = append(violations, "user_goal is required")
	}
	if strings.TrimSpace(a.Mode) == "" {
		violations = append(violations, "mode is required")
	}
	switch strings.TrimSpace(a.MemoryMode) {
	case MemoryModeOff, MemoryModeRelevantOnly, MemoryModeExplicitRecall:
	default:
		violations = append(violations, fmt.Sprintf("memory_mode %q is invalid", a.MemoryMode))
	}
	if len(a.Objectives) != 1 {
		violations = append(violations, "exactly one job-level objective is required")
	}
	seen := map[string]struct{}{}
	actionObjective := false
	for index, objective := range a.Objectives {
		id := strings.TrimSpace(objective.ID)
		if id == "" {
			violations = append(violations, fmt.Sprintf("objectives[%d].id is required", index))
		} else if _, exists := seen[id]; exists {
			violations = append(violations, fmt.Sprintf("objective id %q is duplicated", id))
		} else {
			seen[id] = struct{}{}
		}
		if strings.TrimSpace(objective.Description) == "" {
			violations = append(violations, fmt.Sprintf("objectives[%d].description is required", index))
		}
		if objective.Priority < 1 || objective.Priority > 100 {
			violations = append(violations, fmt.Sprintf("objectives[%d].priority must be between 1 and 100", index))
		}
		criteria := cleanArtifactStrings(objective.AcceptanceCriteria)
		if len(criteria) == 0 {
			violations = append(violations, fmt.Sprintf("objectives[%d].acceptance_criteria is required", index))
		} else if len(criteria) > maxIntentAcceptanceCriteria {
			violations = append(violations, fmt.Sprintf("objectives[%d].acceptance_criteria must contain at most %d items", index, maxIntentAcceptanceCriteria))
		}
		actionObjective = actionObjective || objective.RequiresAction
	}
	if a.RequiresAction != actionObjective {
		violations = append(violations, "requires_action must equal the objective action requirements")
	}
	if len(cleanArtifactStrings(a.CompletionCriteria)) == 0 {
		violations = append(violations, "completion_criteria is required")
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("intent artifact rejected: %s", strings.Join(violations, "; "))
	}
	return nil
}

func cleanArtifactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}
