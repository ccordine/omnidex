package artifacts

import (
	"fmt"
	"sort"
	"strings"
)

const (
	maxIntentAcceptanceCriteria = 12
	maxIntentCompletionCriteria = 12
	maxIntentConstraints        = 32
	maxIntentAmbiguities        = 16
	maxIntentProjectedTextBytes = 4096
)

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
		if !exactBoundedArtifactString(objective.Description, maxIntentProjectedTextBytes) {
			violations = append(violations, fmt.Sprintf(
				"objectives[%d].description must contain one exact value of at most %d bytes",
				index, maxIntentProjectedTextBytes,
			))
		}
		if objective.Priority < 1 || objective.Priority > 100 {
			violations = append(violations, fmt.Sprintf("objectives[%d].priority must be between 1 and 100", index))
		}
		if len(objective.AcceptanceCriteria) == 0 {
			violations = append(violations, fmt.Sprintf("objectives[%d].acceptance_criteria is required", index))
		} else if len(objective.AcceptanceCriteria) > maxIntentAcceptanceCriteria {
			violations = append(violations, fmt.Sprintf("objectives[%d].acceptance_criteria must contain at most %d items", index, maxIntentAcceptanceCriteria))
		} else if !exactUniqueBoundedArtifactStrings(objective.AcceptanceCriteria, maxIntentProjectedTextBytes) {
			violations = append(violations, fmt.Sprintf(
				"objectives[%d].acceptance_criteria must contain exact unique values of at most %d bytes",
				index, maxIntentProjectedTextBytes,
			))
		}
		actionObjective = actionObjective || objective.RequiresAction
	}
	if a.RequiresAction != actionObjective {
		violations = append(violations, "requires_action must equal the objective action requirements")
	}
	if len(a.CompletionCriteria) == 0 {
		violations = append(violations, "completion_criteria is required")
	} else if len(a.CompletionCriteria) > maxIntentCompletionCriteria {
		violations = append(violations, fmt.Sprintf("completion_criteria must contain at most %d items", maxIntentCompletionCriteria))
	} else if !exactUniqueBoundedArtifactStrings(a.CompletionCriteria, maxIntentProjectedTextBytes) {
		violations = append(violations, fmt.Sprintf(
			"completion_criteria must contain exact unique values of at most %d bytes",
			maxIntentProjectedTextBytes,
		))
	}
	if len(a.Constraints) > maxIntentConstraints {
		violations = append(violations, fmt.Sprintf("constraints must contain at most %d items", maxIntentConstraints))
	} else if !exactUniqueBoundedArtifactStrings(a.Constraints, maxIntentProjectedTextBytes) {
		violations = append(violations, fmt.Sprintf(
			"constraints must contain exact unique values of at most %d bytes",
			maxIntentProjectedTextBytes,
		))
	}
	if len(a.Ambiguities) > maxIntentAmbiguities {
		violations = append(violations, fmt.Sprintf("ambiguities must contain at most %d items", maxIntentAmbiguities))
	} else if !exactUniqueBoundedArtifactStrings(a.Ambiguities, maxIntentProjectedTextBytes) {
		violations = append(violations, fmt.Sprintf(
			"ambiguities must contain exact unique values of at most %d bytes",
			maxIntentProjectedTextBytes,
		))
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("intent artifact rejected: %s", strings.Join(violations, "; "))
	}
	return nil
}

func exactUniqueBoundedArtifactStrings(values []string, maxBytes int) bool {
	seen := map[string]struct{}{}
	for _, value := range values {
		if !exactBoundedArtifactString(value, maxBytes) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func exactBoundedArtifactString(value string, maxBytes int) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		!strings.ContainsRune(value, '\x00') && (maxBytes <= 0 || len([]byte(value)) <= maxBytes)
}
