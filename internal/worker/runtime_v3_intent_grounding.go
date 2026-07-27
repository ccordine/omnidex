package worker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

func validateV3IntentGrounding(input v3IntentInput, intent artifacts.IntentArtifact) error {
	authority := strings.Join(append(
		[]string{input.CurrentInstruction, encodeIntentTaskContext(input.TaskContext)},
		input.AuthorityDirectives...,
	), "\n")
	authority = strings.ToLower(authority)
	violations := make([]string, 0)
	for _, text := range intentGroundedTexts(intent) {
		for _, path := range filePathTokenPattern.FindAllString(text, -1) {
			if !strings.Contains(authority, strings.ToLower(path)) {
				violations = append(violations, fmt.Sprintf("intent invented concrete path %q that is absent from current authority", path))
			}
		}
	}
	for objectiveIndex, objective := range intent.Objectives {
		for criterionIndex, criterion := range objective.AcceptanceCriteria {
			if isImplementationGlobalConstraint(criterion) {
				violations = append(violations, fmt.Sprintf(
					"global implementation constraint at objectives[%d].acceptance_criteria[%d] %q must be in constraints, not acceptance_criteria: delete this exact array element and keep the requirement only in top-level constraints",
					objectiveIndex, criterionIndex, criterion,
				))
			}
		}
	}
	for criterionIndex, criterion := range intent.CompletionCriteria {
		if isImplementationGlobalConstraint(criterion) {
			violations = append(violations, fmt.Sprintf(
				"global implementation constraint at completion_criteria[%d] %q must be in constraints, not completion_criteria: delete this exact array element and keep the requirement only in top-level constraints",
				criterionIndex, criterion,
			))
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("v3 intent grounding rejected: %s", strings.Join(violations, "; "))
}

func intentGroundedTexts(intent artifacts.IntentArtifact) []string {
	texts := append([]string(nil), intent.Constraints...)
	texts = append(texts, intent.CompletionCriteria...)
	for _, objective := range intent.Objectives {
		texts = append(texts, objective.Description)
		texts = append(texts, objective.AcceptanceCriteria...)
	}
	return texts
}

func encodeIntentTaskContext(context map[string]any) string {
	if len(context) == 0 {
		return ""
	}
	raw, err := json.Marshal(context)
	if err != nil {
		return ""
	}
	return string(raw)
}

func isImplementationGlobalConstraint(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return false
	}
	if strings.Contains(text, "separat") &&
		strings.Contains(text, "domain") &&
		strings.Contains(text, "storage") &&
		(strings.Contains(text, "command") || strings.Contains(text, "interface")) {
		return true
	}
	if strings.Contains(text, "standard library") {
		return true
	}
	for _, dependencyMarker := range []string{"dependenc", "external librar", "third-party", "framework"} {
		if !strings.Contains(text, dependencyMarker) {
			continue
		}
		for _, restriction := range []string{"only", "no ", "without", "must not", "do not"} {
			if strings.Contains(text, restriction) {
				return true
			}
		}
	}
	return false
}
